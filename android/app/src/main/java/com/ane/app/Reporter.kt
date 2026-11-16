package com.ane.app

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException
import java.util.concurrent.TimeUnit

object Reporter {
    private val mutex = Mutex()
    // Never forward bearer credentials to a redirect destination.
    private val client = OkHttpClient.Builder().connectTimeout(10, TimeUnit.SECONDS).readTimeout(15, TimeUnit.SECONDS)
        .callTimeout(25, TimeUnit.SECONDS).followRedirects(false).followSslRedirects(false).build()
    private data class Result(val status: Int, val body: JSONObject)
    private fun post(path: String, data: JSONObject, config: AppConfig): Result {
        require(config.serverUrl.isNotBlank() && config.deviceToken.length == 64) { "请先保存服务端地址和 64 位设备令牌" }
        val req = Request.Builder().url(config.serverUrl.trimEnd('/') + path)
            .header("Authorization", "Bearer ${config.deviceToken}")
            .header("X-ANE-Device-ID", Telemetry.installationId())
            .post(data.toString().toRequestBody("application/json; charset=utf-8".toMediaType())).build()
        return client.newCall(req).execute().use { response ->
            val source = response.body?.source() ?: throw IOException("服务端返回空响应")
            source.request(65537)
            if (source.buffer.size > 65536) throw IOException("服务端响应过大")
            val body = runCatching { JSONObject(source.readUtf8()) }.getOrElse { throw IOException("服务端返回非 JSON 响应（HTTP ${response.code}）") }
            Result(response.code, body)
        }
    }
    suspend fun heartbeat(): String = withContext(Dispatchers.IO) {
        mutex.withLock {
            val config = SharedStore.state.value.config
            try {
                val result = post("/api/device/heartbeat", Telemetry.snapshot(), config)
                check(result.status in 200..299 && result.body.optInt("code") == 1) { result.body.optString("msg", "连接失败：HTTP ${result.status}") }
                SharedStore.update { it.copy(lastHeartbeat = System.currentTimeMillis(), connection = "连接正常", logs = if (it.connection != "连接正常") it.logs + LogEntry(System.currentTimeMillis(), "服务端心跳连接成功") else it.logs) }
                "连接成功 · 商户 #${result.body.optLong("merchant_id")}"
            } catch (e: Exception) {
                runCatching { SharedStore.update { it.copy(connection = e.message ?: "连接失败", logs = if (it.connection != e.message) it.logs + LogEntry(System.currentTimeMillis(), "连接异常：${e.message}") else it.logs) } }
                throw e
            }
        }
    }
    suspend fun drain(force: Boolean = false) = withContext(Dispatchers.IO) {
        mutex.withLock {
            if (!SharedStore.ready() || !SharedStore.state.value.config.enabled) return@withLock
            val pending = SharedStore.state.value.events.filter { it.state == "pending" && (force || it.nextAttemptAt <= System.currentTimeMillis()) }.take(50)
            for (event in pending) {
                val current = SharedStore.state.value.config
                if (!current.enabled) break
                try {
                    val result = post("/api/device/events", JSONObject().put("notification_id", event.notificationId)
                        .put("package", event.packageName).put("text", event.text).put("posted_at", event.postedAt), current)
                    if (result.status in 200..299 && result.body.optInt("code") == 1) {
                        val state = result.body.optString("status", "sent")
                        SharedStore.update { s -> s.copy(events = s.events.map { if (it.id == event.id) it.copy(state = state, result = result.body.optString("reason", if (result.body.optBoolean("duplicate")) "重复通知，服务端已去重" else "服务端已接收"), attempts = it.attempts + 1) else it }) }
                    } else if (result.status == 400 || result.status == 422) {
                        SharedStore.update { s -> s.copy(events = s.events.map { if (it.id == event.id) it.copy(state = "rejected", result = result.body.optString("msg", "通知无效"), attempts = it.attempts + 1) else it }) }
                    } else throw IOException(result.body.optString("msg", "HTTP ${result.status}"))
                } catch (e: Exception) {
                    val delay = (15_000L * (1L shl event.attempts.coerceAtMost(8))).coerceAtMost(3_600_000L)
                    runCatching { SharedStore.update { s -> s.copy(connection = e.message ?: "上报失败", events = s.events.map { if (it.id == event.id) it.copy(result = e.message ?: "上报失败", attempts = it.attempts + 1, nextAttemptAt = System.currentTimeMillis() + delay) else it }) } }
                    break
                }
            }
        }
    }
}
