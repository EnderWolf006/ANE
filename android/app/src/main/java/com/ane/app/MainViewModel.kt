package com.ane.app

import android.app.Application
import android.net.Uri
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

class MainViewModel(application: Application) : AndroidViewModel(application) {
    val messages = MutableSharedFlow<String>()
    val busy = MutableStateFlow(false)
    private fun task(block: suspend () -> String) {
        if (busy.value) return
        viewModelScope.launch {
            busy.value = true
            val message = runCatching { withContext(Dispatchers.IO) { block() } }.getOrElse { it.message ?: "操作失败" }
            busy.value = false
            messages.emit(message)
        }
    }
    fun selectFolder(uri: Uri) = task {
        // Stop the old runtime before switching storage or restoring another configuration.
        Monitor.stop(getApplication())
        SharedStore.choose(uri)
        if (SharedStore.state.value.config.enabled) Monitor.start(getApplication())
        "目录已连接，已读取共享文件中的配置和记录"
    }
    fun save(url: String, token: String, reportIntervalSeconds: Int, transferPending: Boolean = false) = task {
        val normalized = url.trim().trimEnd('/')
        val parsed = normalized.toHttpUrlOrNull()
        require(parsed != null && parsed.encodedPath == "/" && parsed.query == null && parsed.fragment == null && parsed.username.isEmpty() && parsed.password.isEmpty()) { "请输入完整的 HTTP(S) 服务端地址，不要包含路径" }
        require(token.trim().matches(Regex("[a-fA-F0-9]{64}"))) { "设备令牌应为 64 位十六进制字符" }
        require(reportIntervalSeconds in 5..120) { "上报间隔应为 5–120 秒" }
        val current = SharedStore.state.value
        val changed = current.config.serverUrl != normalized || current.config.deviceToken != token.trim()
        val serverChanged = current.config.serverUrl != normalized
        require(!serverChanged || transferPending || current.events.none { it.state == "pending" }) { "仍有待上报通知，请确认迁移到新服务端" }
        SharedStore.update { it.copy(config = it.config.copy(serverUrl = normalized, deviceToken = token.trim(), reportIntervalSeconds = reportIntervalSeconds), connection = if (changed) "尚未测试新连接" else it.connection, logs = it.logs + LogEntry(System.currentTimeMillis(), "服务端连接配置已保存")) }
        "配置已保存到共享目录"
    }
    fun testConnection() = task { Reporter.heartbeat() }
    fun toggle(enabled: Boolean) = task {
        val c = SharedStore.state.value.config
        if (enabled) require(c.serverUrl.isNotBlank() && c.deviceToken.length == 64) { "请先保存服务端配置" }
        SharedStore.update { it.copy(config = it.config.copy(enabled = enabled), logs = it.logs + LogEntry(System.currentTimeMillis(), if (enabled) "已启用后台监听" else "已暂停后台监听")) }
        if (enabled) {
            try { Monitor.start(getApplication()) } catch (e: Exception) { SharedStore.update { it.copy(config = it.config.copy(enabled = false)) }; throw e }
        } else Monitor.stop(getApplication())
        if (enabled) "后台监听已启用，请确认通知访问权限" else "后台监听已暂停，配置和队列已保留"
    }
    fun retry() = task { require(SharedStore.state.value.config.enabled) { "请先启用监听" }; Reporter.drain(force = true); "已尝试重发待上报通知" }
}
