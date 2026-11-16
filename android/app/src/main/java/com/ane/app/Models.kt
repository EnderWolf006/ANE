package com.ane.app

import org.json.JSONArray
import org.json.JSONObject
import java.math.BigDecimal
import java.security.MessageDigest

object NoticeParser {
    const val ALIPAY = "com.eg.android.AlipayGphone"
    const val WECHAT = "com.tencent.mm"
    private val alipay = Regex("(?:通过扫码向你付款|向你付款|成功收款|收款成功|收款|到账)[：:\\s]*[¥￥]?[\\s]*([0-9][0-9.,]*)[\\s]*元")
    private val wechat = Regex("微信(?:支付)?收款[：:\\s]*[¥￥]?[\\s]*([0-9][0-9.,]*)[\\s]*元")
    private val money = Regex("[0-9][0-9.,]*[\\s]*元")
    private val validMoney = Regex("^(0|[1-9][0-9]{0,5})(\\.[0-9]{1,2})?$")
    private val negative = Regex("退款|退还|失败|取消|撤销|未成功|未到账|未收款|未付款|未支付|交易关闭|待确认|待收款|待付款|待支付|付款请求")
    data class Payment(val cents: Long)
    fun supported(packageName: String) = packageName == ALIPAY || packageName == WECHAT
    fun label(packageName: String) = if (packageName == WECHAT) "微信支付" else "支付宝"
    fun parse(packageName: String, text: String): Payment? {
        if (packageName == ALIPAY && text.contains("微信")) return null
        if (!supported(packageName) || negative.containsMatchIn(text) || money.findAll(text).count() != 1) return null
        val matches = (if (packageName == WECHAT) wechat else alipay).findAll(text).toList()
        if (matches.size != 1) return null
        val amount = matches.single().groupValues[1]
        if (!validMoney.matches(amount)) return null
        val cents = runCatching { BigDecimal(amount).movePointRight(2).longValueExact() }.getOrNull() ?: return null
        return if (cents > 0) Payment(cents) else null
    }
    fun receipt(packageName: String, cents: Long) = "${label(packageName)}收款 ${cents / 100}.${(cents % 100).toString().padStart(2, '0')} 元"
}

data class AppConfig(val serverUrl: String = "", val deviceToken: String = "", val enabled: Boolean = false, val reportIntervalSeconds: Int = 15)
data class PaymentEvent(
    val id: String, val notificationId: String, val text: String, val postedAt: Long,
    val cents: Long, val state: String = "pending", val result: String = "等待上报", val attempts: Int = 0,
    val nextAttemptAt: Long = 0, val packageName: String = NoticeParser.ALIPAY
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
            put("package", e.packageName); put("cents", e.cents); put("state", e.state); put("result", e.result); put("attempts", e.attempts); put("next_attempt_at", e.nextAttemptAt)
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
                    e.getString("id"), e.getString("notification_id"), NoticeParser.receipt(e.optString("package", NoticeParser.ALIPAY), e.getLong("cents")), e.getLong("posted_at"), e.getLong("cents"),
                    e.getString("state"), e.getString("result"), e.getInt("attempts"), e.optLong("next_attempt_at", 0), e.optString("package", NoticeParser.ALIPAY)
                ) },
                logs = (0 until logs.length()).map { i -> val l = logs.getJSONObject(i); LogEntry(l.getLong("time"), l.getString("message")) },
                lastHeartbeat = o.optLong("last_heartbeat"), connection = o.optString("connection", "尚未连接")
            )
        }
    }
}
fun sha256(text: String): String = MessageDigest.getInstance("SHA-256").digest(text.toByteArray()).joinToString("") { "%02x".format(it) }
