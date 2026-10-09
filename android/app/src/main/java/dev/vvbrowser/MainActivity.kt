package dev.vvbrowser

import android.annotation.SuppressLint
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
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.webkit.ProxyConfig
import androidx.webkit.ProxyController
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

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        window.attributes.layoutInDisplayCutoutMode =
            WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS
        WindowCompat.setDecorFitsSystemWindows(window, false)

        buildViews()
        setContentView(root)
        hideSystemBars()

        onBackPressedDispatcher.addCallback(this) {
            when {
                customView != null -> customViewCallback?.onCustomViewHidden()
                webView.canGoBack() -> webView.goBack()
                else -> finish()
            }
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
        if (WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) {
            // DMM embeds the Ubitus player without a quality, so it defaults to
            // "mid" (2 Mbps). Reload the player frame asking for "high" (6-8 Mbps).
            WebViewCompat.addDocumentStartJavaScript(webView, HIGH_QUALITY_JS, setOf(UBITUS_ORIGIN))
        }
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
                Mobile.start()
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
        private const val HIGH_QUALITY_JS = """
            if (location.pathname.startsWith('/gungnir/') && location.search &&
                !/[?&]profile\.quality=/.test(location.search)) {
                location.replace(location.href + '&profile.quality=high');
            }
        """
    }
}
