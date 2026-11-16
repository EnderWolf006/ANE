package com.ane.app

import android.Manifest
import android.content.pm.PackageManager
import android.net.wifi.WifiManager
import androidx.core.content.ContextCompat
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.BatteryManager
import android.os.Build
import android.os.SystemClock
import android.provider.Settings
import org.json.JSONObject

object Telemetry {
    private lateinit var app: android.app.Application
    private var deviceId = ""
    fun initialize(application: android.app.Application) {
        app = application
        // Scoped to the signing certificate, Android user and physical device. Survives reinstall
        // with the same signing key; copying shared config to a different phone cannot clone binding.
        val androidId = Settings.Secure.getString(app.contentResolver, Settings.Secure.ANDROID_ID)
        check(!androidId.isNullOrBlank()) { "无法读取系统设备标识" }
        deviceId = sha256("ANE|$androidId")
    }
    fun installationId(): String = deviceId
    @Suppress("DEPRECATION")
    private fun wifiName(): String {
        if (ContextCompat.checkSelfPermission(app, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) return ""
        return runCatching {
            val wifi = app.getSystemService(WifiManager::class.java)
            val ssid = wifi.connectionInfo?.ssid.orEmpty().removeSurrounding("\"")
            if (ssid == WifiManager.UNKNOWN_SSID || ssid == "0x") "" else ssid.take(32)
        }.getOrDefault("")
    }
    fun snapshot(): JSONObject {
        val battery = app.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
        val level = battery?.getIntExtra(BatteryManager.EXTRA_LEVEL, -1) ?: -1
        val scale = battery?.getIntExtra(BatteryManager.EXTRA_SCALE, -1) ?: -1
        val percent = if (level >= 0 && scale > 0) (level * 100 / scale).coerceIn(0, 100) else -1
        val status = battery?.getIntExtra(BatteryManager.EXTRA_STATUS, -1)
        val charging = status == BatteryManager.BATTERY_STATUS_CHARGING || status == BatteryManager.BATTERY_STATUS_FULL
        val connectivity = app.getSystemService(ConnectivityManager::class.java)
        val capabilities = connectivity.getNetworkCapabilities(connectivity.activeNetwork)
        val network = when {
            capabilities == null -> "none"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN) -> "vpn"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
            capabilities.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
            else -> "other"
        }
        return JSONObject().put("version", BuildConfig.VERSION_NAME)
            .put("model", "${Build.MANUFACTURER} ${Build.MODEL}")
            .put("android_version", Build.VERSION.RELEASE)
            .put("wifi_ssid", if (network == "wifi") wifiName() else "")
            .put("battery", percent).put("charging", charging).put("network", network)
            .put("network_validated", capabilities?.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) == true)
            .put("listener_connected", Monitor.listenerConnected).put("service_running", Monitor.serviceRunning)
            .put("pending_events", SharedStore.state.value.events.count { it.state == "pending" })
            .put("uptime_seconds", SystemClock.elapsedRealtime() / 1000)
    }
}
