package com.ane.app

import android.app.Application
import android.content.Context
import androidx.core.content.edit
import androidx.core.net.toUri
import android.content.Intent
import android.net.Uri
import android.os.ParcelFileDescriptor
import androidx.documentfile.provider.DocumentFile
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

/** The only private preference is a URI locator. All user state is in the selected shared folder. */
object SharedStore {
    private lateinit var context: Application
    private var root: DocumentFile? = null
    private var activeSlot = -1
    private val mutable = MutableStateFlow(Snapshot())
    val state = mutable.asStateFlow()
    val storageError = MutableStateFlow<String?>(null)
    val folder = MutableStateFlow<String?>(null)
    private val names = listOf("ane-state-a.json", "ane-state-b.json")
    @Synchronized fun initialize(ctx: Context) {
        context = ctx.applicationContext as Application
        val uri = context.getSharedPreferences("locator", Context.MODE_PRIVATE).getString("tree_uri", null) ?: return
        runCatching { load(uri.toUri(), false) }.onFailure { storageError.value = "共享目录不可访问，请重新选择原目录：${it.message}" }
    }
    @Synchronized fun choose(uri: Uri) {
        context.contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
        load(uri, true)
        context.getSharedPreferences("locator", Context.MODE_PRIVATE).edit(commit = true) { putString("tree_uri", uri.toString()) }
    }
    private fun load(uri: Uri, create: Boolean) {
        val dir = DocumentFile.fromTreeUri(context, uri) ?: error("目录无效")
        check(dir.canRead() && dir.canWrite()) { "需要目录读写权限" }
        var found = false
        val versions = names.mapIndexedNotNull { index, name ->
            val file = dir.findFile(name) ?: return@mapIndexedNotNull null
            found = true
            runCatching {
                val content = context.contentResolver.openInputStream(file.uri)?.bufferedReader()?.use { it.readText() } ?: error("读取失败")
                index to Snapshot.decode(content)
            }.getOrNull()
        }
        check(!found || versions.isNotEmpty()) { "两个状态副本均损坏，已保留原文件，请从备份恢复" }
        check(found || create) { "未找到状态文件" }
        val latest = versions.maxByOrNull { it.second.revision }
        root = dir; activeSlot = latest?.first ?: -1
        mutable.value = latest?.second ?: Snapshot()
        folder.value = uri.toString(); storageError.value = null
        if (latest == null) update { it }
    }
    @Synchronized fun ready() = root != null && storageError.value == null
    @Synchronized fun update(transform: (Snapshot) -> Snapshot) {
        val dir = root ?: error("请先选择共享存储目录")
        val old = mutable.value
        val candidate = transform(old).let { s -> s.copy(revision = old.revision + 1, events = s.events.filter { it.state == "pending" } + s.events.filter { it.state != "pending" }.takeLast(500), logs = s.logs.takeLast(200)) }
        val targetSlot = if (activeSlot == 0) 1 else 0
        try {
            val file = dir.findFile(names[targetSlot]) ?: dir.createFile("application/json", names[targetSlot]) ?: error("创建文件失败")
            val bytes = candidate.encode().toByteArray(Charsets.UTF_8)
            val fd = context.contentResolver.openFileDescriptor(file.uri, "rwt") ?: error("无法写入文件")
            ParcelFileDescriptor.AutoCloseOutputStream(fd).use { stream -> stream.write(bytes); stream.flush(); runCatching { stream.fd.sync() } }
            // Verify the provider actually committed a readable, complete snapshot.
            val verified = context.contentResolver.openInputStream(file.uri)?.bufferedReader()?.use { Snapshot.decode(it.readText()) } ?: error("写入校验失败")
            check(verified.revision == candidate.revision) { "写入校验失败" }
            mutable.value = candidate; activeSlot = targetSlot; storageError.value = null
        } catch (e: Exception) { storageError.value = "共享存储写入失败：${e.message}"; throw e }
    }
    fun log(message: String) = update { it.copy(logs = it.logs + LogEntry(System.currentTimeMillis(), message)) }
    fun record(packageName: String, notificationId: String, text: String, postedAt: Long) {
        val parsed = NoticeParser.parse(packageName, text) ?: return
        val id = sha256("$packageName|$notificationId|$postedAt|${parsed.cents}")
        update { s -> if (s.events.any { it.id == id || (it.packageName == packageName && it.notificationId == notificationId && it.postedAt == postedAt) }) s else s.copy(events = s.events + PaymentEvent(id, notificationId, NoticeParser.receipt(packageName, parsed.cents), postedAt, parsed.cents, packageName = packageName), logs = s.logs + LogEntry(System.currentTimeMillis(), "收到付款通知 ¥${"%.2f".format(parsed.cents / 100.0)}，已存入上报队列")) }
    }
}
