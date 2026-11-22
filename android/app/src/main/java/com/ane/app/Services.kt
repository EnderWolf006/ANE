package com.ane.app

import android.app.Application
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.BroadcastReceiver
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.os.SystemClock
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.util.concurrent.TimeUnit

class AneApplication : Application() {
    override fun onCreate() { super.onCreate(); Telemetry.initialize(this); SharedStore.initialize(this) }
}
object Monitor {
    @Volatile var listenerConnected = false
    @Volatile var serviceRunning = false
    fun start(context: Context) {
        check(SharedStore.ready()) { "请选择可用的共享目录" }
        check(SharedStore.state.value.config.enabled) { "请先启用监听" }
        ContextCompat.startForegroundService(context, Intent(context, KeepAliveService::class.java))
        val request = PeriodicWorkRequestBuilder<RetryWorker>(15, TimeUnit.MINUTES)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build()).build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork("ane-retry", ExistingPeriodicWorkPolicy.KEEP, request)
        NotificationListenerService.requestRebind(ComponentName(context, AlipayListener::class.java))
    }
    fun stop(context: Context) {
        context.stopService(Intent(context, KeepAliveService::class.java))
        WorkManager.getInstance(context).cancelUniqueWork("ane-retry")
    }
}
class AlipayListener : NotificationListenerService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val intake = Mutex()
    override fun onListenerConnected() {
        Monitor.listenerConnected = true
        // Rescan notifications still present after a listener reconnect or a process restart.
        runCatching { activeNotifications?.forEach { onNotificationPosted(it) } }
    }
    override fun onListenerDisconnected() { Monitor.listenerConnected = false; requestRebind(ComponentName(this, AlipayListener::class.java)) }
    override fun onNotificationPosted(sbn: StatusBarNotification?) {
        if (sbn == null || !NoticeParser.supported(sbn.packageName) || !SharedStore.state.value.config.enabled) return
        // Group summaries can repeat receipts already delivered as individual notifications.
        if (sbn.notification.flags and Notification.FLAG_GROUP_SUMMARY != 0) return
        val extras = sbn.notification.extras
        val title = extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty()
        if (sbn.packageName == NoticeParser.WECHAT && !title.contains("微信收款助手") && !title.contains("微信支付")) return
        val candidates = listOfNotNull(extras.getCharSequence(Notification.EXTRA_BIG_TEXT)?.toString(), extras.getCharSequence(Notification.EXTRA_TEXT)?.toString(), extras.getCharSequence(Notification.EXTRA_TITLE)?.toString()) + (extras.getCharSequenceArray(Notification.EXTRA_TEXT_LINES)?.map { it.toString() } ?: emptyList())
        val text = candidates.map { it.trim() }.firstOrNull { NoticeParser.parse(sbn.packageName, it) != null } ?: return
        scope.launch {
            intake.withLock {
                runCatching { SharedStore.record(sbn.packageName, sbn.key, text, sbn.postTime) }
                    .onSuccess { Reporter.drain() }
            }
        }
    }
    override fun onDestroy() { Monitor.listenerConnected = false; scope.cancel(); super.onDestroy() }
}
class KeepAliveService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    override fun onCreate() {
        super.onCreate()
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel("ane-monitor", "ANE 后台监听", NotificationManager.IMPORTANCE_LOW))
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val notification = NotificationCompat.Builder(this, "ane-monitor").setSmallIcon(R.drawable.ane_notification)
            .setContentTitle("ANE 正在监听支付宝与微信收款").setContentText("点击查看连接、待上报通知和运行状态")
            .setContentIntent(open).setOngoing(true).setCategory(NotificationCompat.CATEGORY_SERVICE).build()
        if (Build.VERSION.SDK_INT >= 34) startForeground(1, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE) else startForeground(1, notification)
        Monitor.serviceRunning = true
        scope.launch {
            SharedStore.state.map { it.config.enabled to it.config.reportIntervalSeconds }
                .distinctUntilChanged().collectLatest { (enabled, seconds) ->
                    if (!enabled) { stopSelf(); return@collectLatest }
                    while (isActive) {
                        val started = SystemClock.elapsedRealtime()
                        if (SharedStore.ready()) {
                            runCatching { Reporter.heartbeat() }
                            runCatching { Reporter.drain() }
                        }
                        delay((seconds.coerceIn(5,120) * 1_000L - (SystemClock.elapsedRealtime() - started)).coerceAtLeast(1_000L))
                    }
                }
        }
    }
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (!SharedStore.state.value.config.enabled || !SharedStore.ready()) { stopSelf(); return START_NOT_STICKY }
        return START_STICKY
    }
    override fun onDestroy() { Monitor.serviceRunning = false; scope.cancel(); super.onDestroy() }
    override fun onBind(intent: Intent?): IBinder? = null
}
class RetryWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        if (!SharedStore.ready() || !SharedStore.state.value.config.enabled) return Result.success()
        return runCatching { Reporter.heartbeat(); Reporter.drain(); Result.success() }.getOrElse { Result.retry() }
    }
}
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED && intent.action != Intent.ACTION_MY_PACKAGE_REPLACED) return
        if (SharedStore.ready() && SharedStore.state.value.config.enabled) runCatching { Monitor.start(context) }
    }
}
