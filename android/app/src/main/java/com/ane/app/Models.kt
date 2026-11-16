package com.ane.app

import org.json.JSONArray
import org.json.JSONObject
import java.math.BigDecimal
import java.security.MessageDigest

object NoticeParser {
    private val pattern = Regex("^(.{1,100}?)通过扫码向你付款((?:0|[1-9][0-9]{0,5})(?:\\.[0-9]{1,2})?)元[。！!]?\\s*$")
    data class Payment(val payer: String, val cents: Long)
    fun parse(text: String): Payment? {
        val m = pattern.matchEntire(text.trim()) ?: return null
        val cents = runCatching { BigDecimal(m.groupValues[2]).movePointRight(2).longValueExact() }.getOrNull() ?: return null
        return if (cents > 0) Payment(m.groupValues[1], cents) else null
    }
}

data class AppConfig(val serverUrl: String = "", val deviceToken: String = "", val enabled: Boolean = false, val reportIntervalSeconds: Int = 15)
data class PaymentEvent(
    val id: String, val notificationId: String, val text: String, val postedAt: Long,
    val cents: Long, val state: String = "pending", val result: String = "等待上报", val attempts: Int = 0,
    val nextAttemptAt: Long = 0
)
data class LogEntry(val time: Long, val message: String)
data class Snapshot(
    val revision: Long = 0, val config: AppConfig = AppConfig(), val events: List<PaymentEvent> = emptyList(),
    val logs: List<LogEntry> = emptyList(), val lastHeartbeat: Long = 0, val connection: String = "尚未连接"
) {
    fun encode(): String = JSONObject().apply {
        put("schema", 2); put("revision", revision)
        put("config", JSONObject().apply { put("server_url", config.serverUrl); put("device_token", config.deviceToken); put("enabled", config.enabled); put("report_interval_seconds", config.reportIntervalSeconds) })
        put("events", JSONArray().apply { events.forEach { e -> put(JSONObject().apply {
            put("id", e.id); put("notification_id", e.notificationId); put("text", e.text); put("posted_at", e.postedAt)
            put("cents", e.cents); put("state", e.state); put("result", e.result); put("attempts", e.attempts); put("next_attempt_at", e.nextAttemptAt)
        }) } })
        put("logs", JSONArray().apply { logs.forEach { l -> put(JSONObject().put("time", l.time).put("message", l.message)) } })
        put("last_heartbeat", lastHeartbeat); put("connection", connection)
    }.toString(2)
    companion object {
        fun decode(text: String): Snapshot {
            val o = JSONObject(text)
            require(o.getInt("schema") == 2) { "不支持的存储版本" }
            val c = o.getJSONObject("config")
            val events = o.getJSONArray("events")
            val logs = o.getJSONArray("logs")
            return Snapshot(
                revision = o.getLong("revision"),
                config = AppConfig(c.getString("server_url"), c.getString("device_token"), c.getBoolean("enabled"), c.optInt("report_interval_seconds", 15).coerceIn(5,120)),
                events = (0 until events.length()).map { i -> val e = events.getJSONObject(i); PaymentEvent(
                    e.getString("id"), e.getString("notification_id"), e.getString("text"), e.getLong("posted_at"), e.getLong("cents"),
                    e.getString("state"), e.getString("result"), e.getInt("attempts"), e.optLong("next_attempt_at", 0)
                ) },
                logs = (0 until logs.length()).map { i -> val l = logs.getJSONObject(i); LogEntry(l.getLong("time"), l.getString("message")) },
                lastHeartbeat = o.optLong("last_heartbeat"), connection = o.optString("connection", "尚未连接")
            )
        }
    }
}
fun sha256(text: String): String = MessageDigest.getInstance("SHA-256").digest(text.toByteArray()).joinToString("") { "%02x".format(it) }
