package dev.vvbrowser

import android.annotation.SuppressLint
import android.app.AlertDialog
import android.content.pm.ApplicationInfo
import android.graphics.Color
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.Gravity
import android.view.View
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.view.WindowManager
import android.webkit.CookieManager
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebChromeClient
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.addCallback
import androidx.core.content.edit
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.webkit.ProxyConfig
import androidx.webkit.ProxyController
import androidx.webkit.ScriptHandler
import androidx.webkit.WebViewCompat
import androidx.webkit.WebViewFeature
import dev.vvbrowser.mobile.Logger
import dev.vvbrowser.mobile.Mobile
import java.util.concurrent.Executors

class MainActivity : ComponentActivity() {

    private val main = Handler(Looper.getMainLooper())
    private val worker = Executors.newSingleThreadExecutor()

    private lateinit var root: FrameLayout
    private lateinit var webView: WebView
    private lateinit var statusPanel: LinearLayout
    private lateinit var status: TextView
    private lateinit var detail: TextView
    private lateinit var retry: Button

    private var connecting = false
    private var customView: View? = null
    private var customViewCallback: WebChromeClient.CustomViewCallback? = null
    private var pageScript: ScriptHandler? = null

    private val prefs by lazy { getSharedPreferences("settings", MODE_PRIVATE) }
    private val quality get() = prefs.getString(PREF_QUALITY, null) ?: Quality.HIGH.param
    private val hideBars get() = prefs.getBoolean(PREF_HIDE_BARS, true)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        window.attributes.layoutInDisplayCutoutMode =
            WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS
        WindowCompat.setDecorFitsSystemWindows(window, false)

        buildViews()
        setContentView(root)
        hideSystemBars()

        // Back opens the in-game menu, so nothing has to sit on top of the game.
        onBackPressedDispatcher.addCallback(this) {
            if (customView != null) customViewCallback?.onCustomViewHidden() else showMenu()
        }

        if (!WebViewFeature.isFeatureSupported(WebViewFeature.PROXY_OVERRIDE)) {
            showError(getString(R.string.error_webview_too_old))
            return
        }
        Mobile.setLogger(object : Logger {
            override fun log(level: String, message: String) = onCoreLog(level, message)
        })
        startCore()
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun buildViews() {
        val debuggable = applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0
        WebView.setWebContentsDebuggingEnabled(debuggable)

        webView = WebView(this).apply {
            setBackgroundColor(Color.BLACK)
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.mediaPlaybackRequiresUserGesture = false
            // The DMM page is laid out for desktop; fit it to the screen.
            settings.useWideViewPort = true
            settings.loadWithOverviewMode = true
            settings.setSupportZoom(false)
            settings.builtInZoomControls = false
            webViewClient = GameWebViewClient()
            webChromeClient = FullscreenChromeClient()
        }
        installPageScript()
        CookieManager.getInstance().apply {
            setAcceptCookie(true)
            // The Ubitus player is embedded from a different site than DMM.
            setAcceptThirdPartyCookies(webView, true)
        }

        status = TextView(this).apply {
            setTextColor(Color.WHITE)
            textSize = 20f
            gravity = Gravity.CENTER
        }
        detail = TextView(this).apply {
            setTextColor(Color.LTGRAY)
            textSize = 13f
            gravity = Gravity.CENTER
            setPadding(0, 24, 0, 24)
        }
        retry = Button(this).apply {
            text = getString(R.string.retry)
            visibility = View.GONE
            setOnClickListener { startCore() }
        }
        statusPanel = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            setBackgroundColor(Color.BLACK)
            setPadding(48, 48, 48, 48)
            addView(status, LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT))
            addView(detail, LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT))
            addView(retry, LinearLayout.LayoutParams(WRAP_CONTENT, WRAP_CONTENT))
        }
        root = FrameLayout(this).apply {
            setBackgroundColor(Color.BLACK)
            addView(webView, FrameLayout.LayoutParams(MATCH_PARENT, MATCH_PARENT))
            addView(statusPanel, FrameLayout.LayoutParams(MATCH_PARENT, MATCH_PARENT))
        }
    }

    /** Starts the local proxy, points WebView at it, then brings the tunnel up. */
    private fun startCore() {
        connecting = true
        statusPanel.visibility = View.VISIBLE
        retry.visibility = View.GONE
        status.text = getString(R.string.status_connecting)
        detail.text = ""

        worker.execute {
            val addr = try {
                Mobile.start(filesDir.absolutePath)
            } catch (e: Exception) {
                main.post { showError(e.message ?: e.toString()) }
                return@execute
            }
            val config = ProxyConfig.Builder().addProxyRule("http://$addr").build()
            ProxyController.getInstance().setProxyOverride(config, worker) {
                try {
                    Mobile.connect()
                    main.post { onConnected() }
                } catch (e: Exception) {
                    main.post { showError(e.message ?: e.toString()) }
                }
            }
        }
    }

    private fun onConnected() {
        connecting = false
        statusPanel.visibility = View.GONE
        if (webView.url == null) {
            webView.loadUrl(GAME_URL)
        }
    }

    /**
     * Injects the page tweaks: the stream quality for the Ubitus player, and
     * optionally hiding DMM's header and footer so the player fills the screen.
     */
    private fun installPageScript() {
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) return
        pageScript?.remove()
        pageScript = WebViewCompat.addDocumentStartJavaScript(
            webView, pageScript(quality, hideBars), setOf(UBITUS_ORIGIN, DMM_ORIGIN),
        )
    }

    private fun showMenu() {
        val items = arrayOf(
            getString(R.string.menu_resume),
            getString(R.string.menu_reload),
            getString(R.string.menu_switch_relay),
            getString(R.string.menu_quality, getString(Quality.of(quality).label)),
            getString(if (hideBars) R.string.menu_show_bars else R.string.menu_hide_bars),
            getString(R.string.menu_exit),
        )
        AlertDialog.Builder(this, android.R.style.Theme_DeviceDefault_Dialog_Alert)
            .setItems(items) { _, which ->
                when (which) {
                    1 -> reloadGame()
                    2 -> switchRelay()
                    3 -> showQualityPicker()
                    4 -> {
                        prefs.edit { putBoolean(PREF_HIDE_BARS, !hideBars) }
                        installPageScript()
                        reloadGame()
                    }
                    5 -> finish()
                }
            }
            .show()
    }

    private fun showQualityPicker() {
        val options = Quality.entries
        AlertDialog.Builder(this, android.R.style.Theme_DeviceDefault_Dialog_Alert)
            .setTitle(R.string.quality_title)
            .setSingleChoiceItems(
                options.map { getString(it.description) }.toTypedArray(),
                options.indexOf(Quality.of(quality)),
            ) { dialog, which ->
                dialog.dismiss()
                if (options[which].param != quality) {
                    prefs.edit { putString(PREF_QUALITY, options[which].param) }
                    installPageScript()
                    reloadGame()
                }
            }
            .show()
    }

    /**
     * Moves to the next best relay. This changes the IP DMM sees, so it is
     * meant for a relay that has become slow or unreliable.
     */
    private fun switchRelay() {
        connecting = true
        statusPanel.visibility = View.VISIBLE
        retry.visibility = View.GONE
        status.text = getString(R.string.status_switching)
        detail.text = ""
        worker.execute {
            try {
                Mobile.switchRelay()
                main.post {
                    connecting = false
                    statusPanel.visibility = View.GONE
                    reloadGame()
                }
            } catch (e: Exception) {
                main.post { showError(e.message ?: e.toString()) }
            }
        }
    }

    /** Reloads the DMM page, which also starts a new stream session. */
    private fun reloadGame() {
        if (webView.url == null) startCore() else webView.reload()
    }

    private fun showError(message: String) {
        connecting = false
        statusPanel.visibility = View.VISIBLE
        status.text = getString(R.string.status_failed)
        detail.text = message
        retry.visibility = View.VISIBLE
    }

    private fun onCoreLog(level: String, message: String) {
        val priority = when (level) {
            "debug" -> Log.DEBUG
            "warn" -> Log.WARN
            "error", "fatal" -> Log.ERROR
            else -> Log.INFO
        }
        Log.println(priority, TAG, message)
        if (connecting && message.startsWith("tunnel:")) {
            main.post { if (connecting) detail.text = message.removePrefix("tunnel: ") }
        }
    }

    private fun hideSystemBars() {
        WindowInsetsControllerCompat(window, window.decorView).apply {
            hide(WindowInsetsCompat.Type.systemBars())
            systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        }
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (hasFocus) hideSystemBars()
    }

    override fun onResume() {
        super.onResume()
        webView.onResume()
    }

    override fun onPause() {
        CookieManager.getInstance().flush()
        webView.onPause()
        super.onPause()
    }

    override fun onDestroy() {
        root.removeView(webView)
        webView.destroy()
        if (isFinishing) {
            worker.execute { Mobile.stop() }
        }
        worker.shutdown()
        super.onDestroy()
    }

    private inner class GameWebViewClient : WebViewClient() {
        override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
            Log.w(TAG, "WebView renderer gone (crash=${detail.didCrash()}), restarting")
            recreate()
            return true
        }
    }

    /** Lets the page's own fullscreen button cover the whole screen. */
    private inner class FullscreenChromeClient : WebChromeClient() {
        override fun onShowCustomView(view: View, callback: CustomViewCallback) {
            if (customView != null) {
                callback.onCustomViewHidden()
                return
            }
            customView = view
            customViewCallback = callback
            root.addView(view, FrameLayout.LayoutParams(MATCH_PARENT, MATCH_PARENT))
            hideSystemBars()
        }

        override fun onHideCustomView() {
            customView?.let { root.removeView(it) }
            customView = null
            customViewCallback = null
            hideSystemBars()
        }
    }

    companion object {
        private const val TAG = "vvbrowser"
        const val GAME_URL = "https://play-cloud.games.dmm.com/cloudgame/gameplay/doaxvv"
        private const val UBITUS_ORIGIN = "https://dcgp-game.ugamenow.com"
        private const val DMM_ORIGIN = "https://play-cloud.games.dmm.com"
        private const val PREF_QUALITY = "quality"
        private const val PREF_HIDE_BARS = "hide_bars"

        /**
         * DMM embeds the Ubitus player without a quality, so it defaults to
         * "mid"; the player frame is reloaded with the chosen one. DMM's page
         * reserves 80px for its header and footer, given back to the player
         * when the bars are hidden.
         */
        private fun pageScript(quality: String, hideBars: Boolean) = """
            (function () {
              if (location.hostname === 'dcgp-game.ugamenow.com') {
                if (location.pathname.startsWith('/gungnir/') && location.search &&
                    !/[?&]profile\.quality=/.test(location.search)) {
                  location.replace(location.href + '&profile.quality=$quality');
                }
              } else if ($hideBars && location.hostname === 'play-cloud.games.dmm.com') {
                var style = document.createElement('style');
                style.textContent =
                  '.header, footer { display: none !important; }' +
                  '.screen { height: 100vh !important; }' +
                  '.screen__wrapper { height: 100% !important; }';
                (document.head || document.documentElement).appendChild(style);
              }
            })();
        """.trimIndent()
    }
}

/** Ubitus quality tiers of the desktop720p profile DMM uses for DOAXVV. */
private enum class Quality(val param: String, val label: Int, val description: Int) {
    HIGH("high", R.string.quality_high, R.string.quality_high_detail),
    MID("mid", R.string.quality_mid, R.string.quality_mid_detail),
    LOW("low", R.string.quality_low, R.string.quality_low_detail);

    companion object {
        fun of(param: String) = entries.firstOrNull { it.param == param } ?: HIGH
    }
}
