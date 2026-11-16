package com.ane.app

import org.junit.Assert.*
import org.junit.Test

class SnapshotTest {
    @Test fun roundTripRetainsConfigurationAndOfflineQueue() {
        val original = Snapshot(
            revision = 42,
            config = AppConfig("https://pay.example.com", "a".repeat(64), true, 30),
            events = listOf(PaymentEvent("id", "alipay-key", "张**通过扫码向你付款0.01元", 1700000000000, 1, attempts = 3, nextAttemptAt = 1700000060000), PaymentEvent("id2", "key2", "李四通过扫码向你付款1元", 1700000000001, 100, "matched", "已匹配", 1)),
            logs = listOf(LogEntry(1700000000000, "包含中文的日志")),
            lastHeartbeat = 1700000000002,
            connection = "连接正常"
        )
        assertEquals(original, Snapshot.decode(original.encode()))
    }
    @Test fun legacySnapshotsDefaultToFifteenSeconds() {
        val json = org.json.JSONObject(Snapshot(config = AppConfig("https://pay.example.com", "a".repeat(64), true)).encode())
        json.getJSONObject("config").remove("report_interval_seconds")
        val restored = Snapshot.decode(json.toString())
        assertEquals(15, restored.config.reportIntervalSeconds)
        assertEquals("a".repeat(64), restored.config.deviceToken)
        assertTrue(restored.config.enabled)
        json.getJSONObject("config").put("report_interval_seconds", 999)
        assertEquals(120, Snapshot.decode(json.toString()).config.reportIntervalSeconds)
    }
    @Test fun rejectsTruncatedOrUnknownVersion() {
        listOf("{", "{}", "{\"schema\":999}", Snapshot().encode().dropLast(10)).forEach { text ->
            assertTrue("invalid snapshot accepted", runCatching { Snapshot.decode(text) }.isFailure)
        }
    }
}
