package server

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestOfflineRemindersHourlyAndRecovery(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "hourly", 5)
	dev := device(t, a, m)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := a.db.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO notification_subscriptions(merchant_id,email,topics) VALUES($1,'initial@example.test',ARRAY['device_offline']),($1,'hourly@example.test',ARRAY['device_offline_reminder'])", m.ID)
	exec("UPDATE devices SET last_seen=now()-interval '3 hours' WHERE merchant_id=$1", m.ID)
	maintain := func(app *App) {
		t.Helper()
		if err := app.maintenance(ctx); err != nil {
			t.Fatal(err)
		}
	}
	count := func(topic string, want int) {
		t.Helper()
		var n int
		if err := a.db.QueryRow(ctx, "SELECT count(*) FROM notification_events WHERE merchant_id=$1 AND topic=$2", m.ID, topic).Scan(&n); err != nil || n != want {
			t.Fatalf("%s: count=%d want=%d err=%v", topic, n, want, err)
		}
	}
	maintain(a)
	count("device_offline", 1)
	count("device_offline_reminder", 0)
	exec("UPDATE notification_events SET created_at=now()-interval '59 minutes' WHERE topic='device_offline'")
	maintain(a)
	count("device_offline_reminder", 0)
	exec("UPDATE notification_events SET created_at=now()-interval '61 minutes' WHERE topic='device_offline'")
	// Another worker instance must share the persisted cadence and merchant lock.
	other := &App{db: a.db}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, app := range []*App{a, other} {
		wg.Add(1)
		go func(app *App) { defer wg.Done(); errors <- app.maintenance(ctx) }(app)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	count("device_offline_reminder", 1)
	maintain(other)
	count("device_offline_reminder", 1)
	var recipients int
	if err := a.db.QueryRow(ctx, "SELECT count(*) FROM mail_queue q JOIN notification_events e ON e.id=q.event_id WHERE e.topic='device_offline_reminder' AND q.recipient='hourly@example.test'").Scan(&recipients); err != nil || recipients != 1 {
		t.Fatal("independent subscription", recipients, err)
	}
	var leaked int
	_ = a.db.QueryRow(ctx, "SELECT count(*) FROM mail_queue q JOIN notification_events e ON e.id=q.event_id WHERE e.topic='device_offline_reminder' AND q.recipient='initial@example.test'").Scan(&leaked)
	if leaked != 0 {
		t.Fatal("existing subscription was opted in")
	}
	exec("UPDATE notification_events SET created_at=now()-interval '61 minutes' WHERE topic='device_offline_reminder'")
	maintain(other)
	count("device_offline_reminder", 2)
	var subject, body string
	_ = a.db.QueryRow(ctx, "SELECT title,message FROM notification_events WHERE topic='device_offline_reminder' ORDER BY id DESC LIMIT 1").Scan(&subject, &body)
	if !strings.Contains(subject, "仍然离线") || !strings.Contains(body, "最近心跳:") || !strings.Contains(body, "提醒频率: 每小时一次") {
		t.Fatal(subject, body)
	}
	// Recovery cancels already queued reminders, including before another offline episode.
	exec("UPDATE notification_events SET created_at=now()-interval '10 minutes' WHERE topic='device_offline_reminder' AND created_at>now()-interval '10 minutes'")
	w := request(t, a, "POST", "/api/device/heartbeat", Telemetry{Battery: 80, Network: "wifi"}, dev)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	exec("UPDATE devices SET last_seen=now()-interval '5 minutes' WHERE merchant_id=$1", m.ID)
	maintain(a)
	count("device_offline", 2)
	count("device_offline_reminder", 2)
	exec("UPDATE smtp_settings SET enabled=true")
	sent := 0
	a.sendMail = func(context.Context, SMTPConfig, string, string, string) error { sent++; return nil }
	for i := 0; i < 10 && a.mailOnce(ctx); i++ {
	}
	var cancelled int
	_ = a.db.QueryRow(ctx, "SELECT count(*) FROM mail_queue q JOIN notification_events e ON e.id=q.event_id WHERE e.topic='device_offline_reminder' AND q.status='cancelled'").Scan(&cancelled)
	if cancelled != 2 || sent != 2 {
		t.Fatalf("stale reminders: cancelled=%d sent=%d", cancelled, sent)
	}
	// Disabled merchants and revoked devices do not receive reminders.
	exec("UPDATE notification_events SET created_at=now()-interval '61 minutes'")
	exec("UPDATE merchants SET enabled=false WHERE id=$1", m.ID)
	maintain(a)
	count("device_offline_reminder", 2)
	exec("UPDATE merchants SET enabled=true WHERE id=$1", m.ID)
	exec("UPDATE devices SET revoked=true WHERE merchant_id=$1", m.ID)
	maintain(a)
	count("device_offline_reminder", 2)
}

func TestOfflineReminderWithoutPreviousAlert(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	m := testMerchant(t, a, "legacy_offline", 5)
	device(t, a, m)
	// A legacy offline device with no last_seen or matching event still has a cadence anchor.
	_, err := a.db.Exec(ctx, "UPDATE devices SET created_at=now()-interval '2 hours',offline_alerted=true WHERE merchant_id=$1", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec(ctx, "INSERT INTO notification_subscriptions(merchant_id,email,topics) VALUES($1,'hourly@example.test',ARRAY['device_offline_reminder'])", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.maintenance(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = a.db.QueryRow(ctx, "SELECT count(*) FROM notification_events WHERE topic='device_offline_reminder'").Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	// Rebinding resets created_at and invalidates reminders from the old device.
	_, err = a.db.Exec(ctx, "UPDATE devices SET created_at=now(),last_seen=NULL WHERE merchant_id=$1", m.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = a.db.Exec(ctx, "UPDATE smtp_settings SET enabled=true")
	a.sendMail = func(context.Context, SMTPConfig, string, string, string) error {
		t.Error("sent old device reminder")
		return nil
	}
	if !a.mailOnce(ctx) {
		t.Fatal("queue not consumed")
	}
	var status string
	_ = a.db.QueryRow(ctx, "SELECT status FROM mail_queue LIMIT 1").Scan(&status)
	if status != "cancelled" {
		t.Fatal(status)
	}
}
