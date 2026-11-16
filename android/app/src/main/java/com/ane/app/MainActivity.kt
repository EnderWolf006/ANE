package com.ane.app

import android.Manifest
import android.app.NotificationManager
import android.content.ClipboardManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.net.toUri
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.json.JSONObject
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

private val Green = Color(0xFF17654C)
private val Canvas = Color(0xFFF4F6F3)
private val Muted = Color(0xFF7C8A80)
private val Ink = Color(0xFF1B2D23)
private val Pale = Color(0xFFE7F0E7)
private val colors = lightColorScheme(primary = Green, secondary = Green, background = Canvas, surface = Color.White, onSurface = Ink, onBackground = Ink, surfaceContainer = Pale)

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState); enableEdgeToEdge()
        setContent { MaterialTheme(colorScheme = colors) { AneApp() } }
    }
}
private fun timestamp(time: Long): String = if (time == 0L) "尚无记录" else SimpleDateFormat("MM-dd HH:mm:ss", Locale.CHINA).format(Date(time))
private fun notificationAccess(context: Context): Boolean {
    val flat = Settings.Secure.getString(context.contentResolver, "enabled_notification_listeners") ?: return false
    return flat.split(':').any { ComponentName.unflattenFromString(it)?.packageName == context.packageName }
}
private fun openSetting(context: Context, intent: Intent) {
    runCatching { context.startActivity(intent) }.onFailure {
        context.startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, "package:${context.packageName}".toUri()))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable private fun AneApp(vm: MainViewModel = viewModel()) {
    val context = LocalContext.current
    val snapshot by SharedStore.state.collectAsState()
    val storageError by SharedStore.storageError.collectAsState()
    val folder by SharedStore.folder.collectAsState()
    val busy by vm.busy.collectAsState()
    var tab by remember { mutableIntStateOf(0) }
    var eventLimit by remember { mutableIntStateOf(30) }
    var logLimit by remember { mutableIntStateOf(30) }
    val sortedEvents = remember(snapshot.events) { snapshot.events.sortedByDescending { it.postedAt } }
    val sortedLogs = remember(snapshot.logs) { snapshot.logs.asReversed() }
    var access by remember { mutableStateOf(notificationAccess(context)) }
    var foreground by remember { mutableStateOf(Monitor.serviceRunning) }
    var listener by remember { mutableStateOf(Monitor.listenerConnected) }
    var battery by remember { mutableStateOf(false) }
    var notifications by remember { mutableStateOf(false) }
    var wifiPermission by remember { mutableStateOf(false) }
    var backgroundLocation by remember { mutableStateOf(false) }
    var locationEnabled by remember { mutableStateOf(false) }
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val treePicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri -> if (uri != null) vm.selectFolder(uri) }
    val wifiPermissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { }
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { }
    fun pickFolder() { treePicker.launch(null) }
    LaunchedEffect(Unit) {
        if (SharedStore.ready() && snapshot.config.enabled) runCatching { Monitor.start(context) }
        while (true) {
            access = notificationAccess(context); foreground = Monitor.serviceRunning; listener = Monitor.listenerConnected
            battery = context.getSystemService(PowerManager::class.java).isIgnoringBatteryOptimizations(context.packageName)
            notifications = context.getSystemService(NotificationManager::class.java).areNotificationsEnabled()
            wifiPermission = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
            backgroundLocation = Build.VERSION.SDK_INT < 29 || ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_BACKGROUND_LOCATION) == PackageManager.PERMISSION_GRANTED
            locationEnabled = androidx.core.location.LocationManagerCompat.isLocationEnabled(context.getSystemService(android.location.LocationManager::class.java))
            delay(1500)
        }
    }
    LaunchedEffect(vm) { vm.messages.collect { snackbar.showSnackbar(it) } }
    Scaffold(
        containerColor = Canvas,
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            Column(Modifier.statusBarsPadding().padding(horizontal = 24.dp, vertical = 18.dp)) {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.size(42.dp).background(Green, RoundedCornerShape(13.dp)), contentAlignment = Alignment.Center) { Text("A·", color = Color.White, fontSize = 26.sp, fontWeight = FontWeight.Bold) }
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f)) { Text("ANE", fontSize = 23.sp, fontWeight = FontWeight.Bold, letterSpacing = 2.sp); Text("收款设备", fontSize = 10.sp, color = Muted) }
                    BadgeLabel(if (snapshot.config.enabled && foreground && access) "监听已启用" else "监听未就绪", snapshot.config.enabled && foreground && access)
                }
            }
        },
        bottomBar = {
            NavigationBar(containerColor = Color.White, tonalElevation = 0.dp) {
                listOf("总览" to "◉", "配置" to "⚙", "通知" to "▤", "状态" to "◷").forEachIndexed { index, (title, symbol) ->
                    NavigationBarItem(selected = tab == index, onClick = { tab = index }, icon = { Text(symbol, fontSize = 21.sp) }, label = { Text(title, fontSize = 11.sp) }, colors = NavigationBarItemDefaults.colors(indicatorColor = Pale, selectedIconColor = Green, selectedTextColor = Green))
                }
            }
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            if (busy) LinearProgressIndicator(Modifier.fillMaxWidth(), color = Green)
            if (storageError != null) Text(storageError!!, Modifier.fillMaxWidth().background(Color(0xFFFFECE3)).padding(16.dp), color = Color(0xFF9B573D), fontSize = 12.sp)
            when (tab) {
                0 -> LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    item {
                        Surface(color = Pale, shape = RoundedCornerShape(24.dp)) {
                            Column(Modifier.fillMaxWidth().padding(25.dp)) {
                                Text("YOUR PAYMENTS, CONNECTED.", fontSize = 9.sp, letterSpacing = 1.5.sp, color = Muted)
                                Spacer(Modifier.height(15.dp)); Text("每一笔收款，\n都有迹可循。", fontSize = 28.sp, fontWeight = FontWeight.SemiBold, lineHeight = 40.sp)
                                Spacer(Modifier.height(12.dp)); Text("监听支付宝与微信收款通知，安全连接你的支付服务。", fontSize = 12.sp, color = Muted)
                            }
                        }
                    }
                    if (folder == null) item { AneCard("先连接共享存储") { Text("选择 Documents 中的 ANE 文件夹。配置、通知队列与日志均保存在此目录，卸载后文件仍保留。重装时重新选择同一目录即可恢复。", color = Muted, fontSize = 12.sp); Spacer(Modifier.height(12.dp)); Button(onClick = { pickFolder() }, enabled = !busy) { Text("选择 / 恢复目录") } } }
                    item {
                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            Metric("待上报", snapshot.events.count { it.state == "pending" }.toString(), "离线通知自动重试", Modifier.weight(1f))
                            Metric("已匹配", snapshot.events.count { it.state == "matched" }.toString(), "最近 500 条已处理记录", Modifier.weight(1f))
                        }
                    }
                    item { AneCard("监听开关") { Row(verticalAlignment = Alignment.CenterVertically) { Column(Modifier.weight(1f)) { Text(if (snapshot.config.enabled) "已启用后台收款监听" else "开启自动上报", fontSize = 14.sp, fontWeight = FontWeight.Medium); Text("通知先存盘，再发送至服务端", color = Muted, fontSize = 11.sp) }; Switch(checked = snapshot.config.enabled, onCheckedChange = { vm.toggle(it) }, enabled = !busy && folder != null && storageError == null) }; if (!access) { Spacer(Modifier.height(8.dp)); TextButton(onClick = { openSetting(context, Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS)) }) { Text("授予收款通知访问权限 →") } } } }
                    item { AneCard("连接状态") { InfoRow("服务端", snapshot.config.serverUrl.ifEmpty { "尚未配置" }); InfoRow("连接结果", snapshot.connection); InfoRow("最近心跳", timestamp(snapshot.lastHeartbeat)); Spacer(Modifier.height(10.dp)); OutlinedButton(onClick = { vm.testConnection() }, enabled = !busy && folder != null) { Text("测试服务端连接") } } }
                    item { Text("ANE 根据系统通知匹配订单。支付记录与资金到账请以支付宝或微信账单为准。", color = Muted, fontSize = 10.sp, lineHeight = 17.sp) }
                }
                1 -> ConfigScreen(snapshot, folder, busy, { pickFolder() }, vm) { scope.launch { snackbar.showSnackbar(it) } }
                2 -> LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    item { SectionTitle("收款通知", "待上报通知会保留到成功接收；已处理记录最多保留 500 条。") }
                    item { OutlinedButton(onClick = { vm.retry() }, enabled = !busy && snapshot.events.any { it.state == "pending" }) { Text("重试所有待上报通知") } }
                    if (snapshot.events.isEmpty()) item { AneCard("还没有收款通知") { Text("启用监听后，符合“xx通过扫码向你付款0.01元”的通知将显示在这里。", color = Muted, fontSize = 12.sp) } }
                    items(sortedEvents.take(eventLimit), key = { it.id }) { event ->
                        AneCard("¥${"%.2f".format(Locale.ROOT, event.cents / 100.0)}") {
                            Text(event.text, fontSize = 13.sp); Spacer(Modifier.height(10.dp))
                            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) { Text(timestamp(event.postedAt), color = Muted, fontSize = 10.sp); BadgeLabel(when (event.state) { "pending" -> "待上报"; "matched" -> "已匹配"; "unmatched" -> "未匹配"; "ignored" -> "已忽略"; "rejected" -> "已拒绝"; else -> "已接收" }, event.state == "matched") }
                            Spacer(Modifier.height(8.dp)); Text(event.result, color = Muted, fontSize = 11.sp); if (event.attempts > 0) Text("上报尝试 ${event.attempts} 次", color = Muted, fontSize = 10.sp)
                        }
                    }
                    item(key = "events-more") { LoadMoreFooter(eventLimit, sortedEvents.size) { eventLimit += 30 } }
                }
                3 -> LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                    item { SectionTitle("运行状态", "请完成以下设置，减少系统对后台监听的限制。") }
                    item { AneCard("权限与保活") {
                        PermissionRow("通知访问", if (access) "已授权 · ${if (listener) "监听已连接" else "等待系统连接"}" else "未授权", access) { openSetting(context, Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS)) }
                        PermissionRow("常驻通知", if (notifications) "已允许" else "未允许", notifications) { if (Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) permission.launch(Manifest.permission.POST_NOTIFICATIONS) else openSetting(context, Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName)) }
                        PermissionRow("Wi-Fi 名称", if (!wifiPermission) "需授予精确定位权限" else if (!locationEnabled) "请开启系统定位" else if (!backgroundLocation) "前台已授权 · 后台需始终允许" else "已授权后台读取", wifiPermission && locationEnabled && backgroundLocation) {
                            if (!wifiPermission) wifiPermissionLauncher.launch(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION))
                            else if (!locationEnabled) openSetting(context, Intent(Settings.ACTION_LOCATION_SOURCE_SETTINGS))
                            else if (Build.VERSION.SDK_INT == 29 && !backgroundLocation) permission.launch(Manifest.permission.ACCESS_BACKGROUND_LOCATION)
                            else openSetting(context, Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, "package:${context.packageName}".toUri()))
                        }
                        Text("Android 将 Wi-Fi 名称视为位置信息。仅用于显示当前网络名称，不获取坐标。后台显示需在定位权限中选择“始终允许”，并开启系统定位；未授权不影响收款监听。", color = Muted, fontSize = 11.sp, lineHeight = 18.sp)
                        PermissionRow("电池优化", if (battery) "已允许不受限制" else "仍受系统优化限制", battery) { openSetting(context, Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)) }
                        PermissionRow("前台服务", if (foreground) "正在运行" else "尚未运行", foreground) { vm.toggle(true) }
                        Spacer(Modifier.height(8.dp)); OutlinedButton(onClick = { openSetting(context, Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, "package:${context.packageName}".toUri())) }) { Text("应用设置 / 厂商自启动设置") }
                        Spacer(Modifier.height(10.dp)); Text("建议允许自启动、后台活动，并在最近任务中锁定 ANE。系统强行停止、厂商清理或撤销权限仍可能中断监听；无法保证进程永不被杀。", color = Muted, fontSize = 11.sp, lineHeight = 18.sp)
                    } }
                    item { AneCard("设备遥测") {
                        val telemetry = Telemetry.snapshot()
                        InfoRow("设备电量", if (telemetry.getInt("battery") < 0) "未知" else "${telemetry.getInt("battery")}% · ${if (telemetry.getBoolean("charging")) "正在充电" else "未充电"}")
                        InfoRow("网络", if (telemetry.getString("network") == "wifi") "Wi-Fi · ${telemetry.optString("wifi_ssid").ifBlank { "名称未获取" }}" else telemetry.getString("network"))
                        InfoRow("设备标识", Telemetry.installationId())
                        Text("每 ${snapshot.config.reportIntervalSeconds} 秒上报电量、网络、心跳、监听和队列状态。换机需在 Web 面板重置绑定；相同签名重装会保留系统设备标识。", color = Muted, fontSize = 11.sp, lineHeight = 18.sp)
                    } }
                    item { AneCard("共享存储") { InfoRow("目录", folder ?: "尚未选择"); Text("仅目录定位信息保存在应用内部；配置、令牌、队列和日志保存为 ane-state-a.json / ane-state-b.json 两个轮换副本。", fontSize = 11.sp, color = Muted); Spacer(Modifier.height(10.dp)); OutlinedButton(onClick = { pickFolder() }, enabled = !busy) { Text("重新授权 / 恢复目录") } } }
                    item { SectionTitle("运行日志", "按时间倒序，下滑自动加载更多。") }
                    if (sortedLogs.isEmpty()) item { Text("暂无日志", color = Muted, fontSize = 12.sp) }
                    items(sortedLogs.take(logLimit)) { log -> AneCard(timestamp(log.time)) { Text(log.message, fontSize = 12.sp) } }
                    item(key = "logs-more") { LoadMoreFooter(logLimit, sortedLogs.size) { logLimit += 30 } }
                    item { Text("ANE ${BuildConfig.VERSION_NAME} · Android ${Build.VERSION.RELEASE}", color = Muted, fontSize = 10.sp) }
                }
            }
        }
    }
}

@Composable private fun ConfigScreen(snapshot: Snapshot, folder: String?, busy: Boolean, choose: () -> Unit, vm: MainViewModel, message: (String) -> Unit) {
    val context = LocalContext.current
    var url by remember(snapshot.config.serverUrl) { mutableStateOf(snapshot.config.serverUrl) }
    var token by remember(snapshot.config.deviceToken) { mutableStateOf(snapshot.config.deviceToken) }
    var reportInterval by remember(snapshot.config.reportIntervalSeconds) { mutableStateOf(snapshot.config.reportIntervalSeconds.toString()) }
    var visible by remember { mutableStateOf(false) }
    var confirmTransfer by remember { mutableStateOf(false) }
    if (confirmTransfer) AlertDialog(
        onDismissRequest = { confirmTransfer = false },
        title = { Text("迁移待上报通知？") },
        text = { Text("队列里还有未发送的收款通知。保存后这些通知将发送到新的服务端地址，请确认新地址属于同一个收款后端。原服务端可能已接收过部分通知。") },
        confirmButton = { TextButton(onClick = { confirmTransfer = false; vm.save(url, token, reportInterval.toIntOrNull() ?: 0, transferPending = true) }) { Text("确认迁移并保存") } },
        dismissButton = { TextButton(onClick = { confirmTransfer = false }) { Text("取消") } }
    )
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        item { SectionTitle("连接配置", "每位商户绑定一台设备。在 Web 面板生成连接信息后填入。") }
        item { AneCard("存储位置") { Text(if (folder != null) "已连接共享目录。保存后可在卸载重装时恢复。" else "请先选择共享目录，配置将直接写入此处。", color = Muted, fontSize = 12.sp); Spacer(Modifier.height(10.dp)); OutlinedButton(onClick = choose, enabled = !busy) { Text(if (folder != null) "选择 / 恢复其他目录" else "选择共享目录") } } }
        item { AneCard("服务端连接") {
            OutlinedTextField(value = url, onValueChange = { url = it }, label = { Text("服务端地址") }, placeholder = { Text("https://pay.example.com") }, singleLine = true, modifier = Modifier.fillMaxWidth(), keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri))
            Spacer(Modifier.height(16.dp))
            OutlinedTextField(value = token, onValueChange = { token = it }, label = { Text("设备令牌") }, singleLine = true, modifier = Modifier.fillMaxWidth(), visualTransformation = if (visible) VisualTransformation.None else PasswordVisualTransformation(), trailingIcon = { TextButton(onClick = { visible = !visible }) { Text(if (visible) "隐藏" else "显示", fontSize = 10.sp) } })
            TextButton(onClick = {
                runCatching {
                    val clipboard = context.getSystemService(ClipboardManager::class.java)
                    val data = JSONObject(clipboard.primaryClip?.getItemAt(0)?.coerceToText(context)?.toString() ?: error("剪贴板为空"))
                    url = data.getString("server_url"); token = data.getString("device_token")
                }.onSuccess { message("连接信息已填入，请检查并保存") }.onFailure { message("请复制 Web 面板中的 JSON 连接信息") }
            }) { Text("从剪贴板导入连接信息", fontSize = 12.sp) }
            OutlinedTextField(value = reportInterval, onValueChange = { reportInterval = it.filter(Char::isDigit).take(3) }, label = { Text("状态上报间隔（秒）") }, supportingText = { Text("默认 15 秒，可配置 5–120 秒；收款通知仍立即上报。") }, singleLine = true, modifier = Modifier.fillMaxWidth(), keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number))
            Spacer(Modifier.height(8.dp))
            Button(onClick = { if (url.trim().trimEnd('/') != snapshot.config.serverUrl && snapshot.events.any { it.state == "pending" }) confirmTransfer = true else vm.save(url, token, reportInterval.toIntOrNull() ?: 0) }, enabled = !busy && folder != null, modifier = Modifier.fillMaxWidth()) { Text("保存到共享目录") }
            OutlinedButton(onClick = { vm.testConnection() }, enabled = !busy && snapshot.config.serverUrl.isNotEmpty(), modifier = Modifier.fillMaxWidth()) { Text("测试已保存的连接") }
            Text("手机上的 localhost 指手机本身。局域网调试请填写电脑的局域网 IP，并开放服务监听端口。正式部署请使用 HTTPS。", color = Muted, fontSize = 11.sp, lineHeight = 18.sp)
        } }
        item { Surface(color = Color(0xFFFAF0DF), shape = RoundedCornerShape(16.dp)) { Text("共享文件中含设备令牌及收款通知。请使用你控制的本地目录，勿共享给他人。卸载会清除 Android 权限，重装后需重新选择原目录并授予通知访问权限。", Modifier.padding(18.dp), fontSize = 11.sp, color = Color(0xFF987947), lineHeight = 19.sp) } }
    }
}
@Composable private fun SectionTitle(title: String, subtitle: String) { Column { Text(title, fontSize = 24.sp, fontWeight = FontWeight.SemiBold); Spacer(Modifier.height(8.dp)); Text(subtitle, fontSize = 12.sp, color = Muted, lineHeight = 19.sp) } }
@Composable private fun AneCard(title: String, content: @Composable ColumnScope.() -> Unit) { Surface(shape = RoundedCornerShape(20.dp), color = Color.White, modifier = Modifier.fillMaxWidth()) { Column(Modifier.padding(21.dp)) { Text(title, fontSize = 15.sp, fontWeight = FontWeight.SemiBold); Spacer(Modifier.height(15.dp)); content() } } }
@Composable private fun Metric(title: String, value: String, caption: String, modifier: Modifier) { Surface(modifier = modifier, shape = RoundedCornerShape(20.dp), color = Color.White) { Column(Modifier.padding(19.dp)) { Text(title, fontSize = 12.sp, color = Muted); Text(value, fontSize = 34.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(vertical = 5.dp)); Text(caption, fontSize = 9.sp, color = Muted) } } }
@Composable private fun BadgeLabel(label: String, good: Boolean) { Surface(color = if (good) Pale else Color(0xFFF4F0E7), shape = RoundedCornerShape(7.dp)) { Text(label, Modifier.padding(horizontal = 9.dp, vertical = 5.dp), fontSize = 10.sp, color = if (good) Green else Color(0xFF9A835A)) } }
@Composable private fun InfoRow(label: String, value: String) { Column(Modifier.padding(bottom = 14.dp)) { Text(label, color = Muted, fontSize = 10.sp); Spacer(Modifier.height(4.dp)); Text(value, fontSize = 12.sp, lineHeight = 19.sp) } }
@Composable private fun PermissionRow(title: String, description: String, good: Boolean, action: () -> Unit) { Row(Modifier.fillMaxWidth().padding(vertical = 11.dp), verticalAlignment = Alignment.CenterVertically) { Column(Modifier.weight(1f)) { Text(title, fontSize = 13.sp, fontWeight = FontWeight.Medium); Text(description, color = Muted, fontSize = 10.sp) }; if (good) Text("✓", color = Green, fontSize = 18.sp) else TextButton(onClick = action) { Text("设置 →", fontSize = 12.sp) } } }

@Composable private fun LoadMoreFooter(visible: Int, total: Int, loadMore: () -> Unit) {
    if (total == 0) return
    if (visible < total) {
        LaunchedEffect(visible, total) { delay(150); loadMore() }
        Row(Modifier.fillMaxWidth().padding(16.dp), horizontalArrangement = Arrangement.Center, verticalAlignment = Alignment.CenterVertically) {
            CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
            Spacer(Modifier.width(10.dp)); Text("正在加载更多 · 已显示 ${minOf(visible,total)} / $total", fontSize = 11.sp, color = Muted)
        }
    } else Text("已显示全部 $total 条记录", Modifier.fillMaxWidth().padding(16.dp), fontSize = 11.sp, color = Muted)
}
