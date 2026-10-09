// Prevents an extra console window on Windows in release builds.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::error::Error;
use std::net::TcpListener;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Mutex;

use serde::{Deserialize, Serialize};
use tauri::menu::{CheckMenuItem, IsMenuItem, Menu, MenuEvent, MenuItem, PredefinedMenuItem, Submenu};
use tauri::{AppHandle, Emitter, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder, Wry};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;
use url::Url;

const GAME_URL: &str = "https://play-cloud.games.dmm.com/cloudgame/gameplay/doaxvv";

/// Ubitus quality tiers of the desktop720p profile DMM uses for DOAXVV.
const QUALITIES: [(&str, &str); 3] = [
    ("high", "High: 1280×720, 6–8 Mbps"),
    ("mid", "Medium: 1280×720, 2 Mbps"),
    ("low", "Low: 960×540, 2 Mbps"),
];

#[derive(Serialize, Deserialize)]
#[serde(default)]
struct Settings {
    quality: String,
    hide_bars: bool,
}

impl Default for Settings {
    fn default() -> Self {
        Self { quality: "high".into(), hide_bars: true }
    }
}

impl Settings {
    fn path(app: &AppHandle) -> Option<PathBuf> {
        app.path().app_config_dir().ok().map(|d| d.join("settings.json"))
    }

    fn load(app: &AppHandle) -> Self {
        Self::path(app)
            .and_then(|p| std::fs::read(p).ok())
            .and_then(|b| serde_json::from_slice(&b).ok())
            .unwrap_or_default()
    }

    fn save(&self, app: &AppHandle) {
        if let Some(path) = Self::path(app) {
            if let Some(dir) = path.parent() {
                let _ = std::fs::create_dir_all(dir);
            }
            if let Ok(json) = serde_json::to_vec_pretty(self) {
                let _ = std::fs::write(path, json);
            }
        }
    }

    /// Page tweaks injected into every frame. DMM embeds the Ubitus player
    /// without a quality, so it defaults to "mid"; the player frame is
    /// reloaded with the chosen one. DMM's page reserves 80px for its header
    /// and footer, given back to the player when the bars are hidden.
    fn page_script(&self) -> String {
        let quality = QUALITIES
            .iter()
            .find(|(id, _)| *id == self.quality)
            .map_or("high", |(id, _)| id);
        format!(
            r#"(function () {{
  if (location.hostname === 'dcgp-game.ugamenow.com') {{
    if (location.pathname.startsWith('/gungnir/') && location.search &&
        !/[?&]profile\.quality=/.test(location.search)) {{
      location.replace(location.href + '&profile.quality={quality}');
    }}
  }} else if ({hide_bars} && location.hostname === 'play-cloud.games.dmm.com') {{
    var style = document.createElement('style');
    style.textContent =
      '.header, footer {{ display: none !important; }}' +
      '.screen {{ height: 100vh !important; }}' +
      '.screen__wrapper {{ height: 100% !important; }}';
    (document.head || document.documentElement).appendChild(style);
  }}
}})();"#,
            hide_bars = self.hide_bars,
        )
    }
}

/// The running network core (the `vvcore` sidecar).
struct Core {
    port: u16,
    /// The loading screen, used to come back to it on errors.
    loading_url: Url,
    child: Mutex<Option<CommandChild>>,
    /// Bumped on every (re)start so events from an old process are ignored.
    generation: AtomicU64,
}

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![retry])
        .on_menu_event(on_menu_event)
        .setup(|app| {
            let port = free_port()?;
            let settings = Settings::load(app.handle());
            let window = WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
                .title("VV Browser")
                .inner_size(1280.0, 800.0)
                .min_inner_size(640.0, 400.0)
                .menu(build_menu(app.handle(), &settings)?)
                .proxy_url(Url::parse(&format!("http://127.0.0.1:{port}"))?)
                .initialization_script_for_all_frames(settings.page_script())
                .build()?;
            app.manage(Core {
                port,
                loading_url: window.url()?,
                child: Mutex::new(None),
                generation: AtomicU64::new(0),
            });
            start_core(app.handle())?;
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building VV Browser");

    app.run(|app, event| {
        if let RunEvent::Exit = event {
            stop_core(app);
        }
    });
}

/// Restarts the network core after a failure; called from the loading screen.
#[tauri::command]
fn retry(app: AppHandle) -> Result<(), String> {
    start_core(&app).map_err(|e| e.to_string())
}

fn build_menu(app: &AppHandle, settings: &Settings) -> tauri::Result<Menu<Wry>> {
    let qualities = QUALITIES
        .iter()
        .map(|(id, label)| {
            CheckMenuItem::with_id(app, format!("quality:{id}"), label, true, settings.quality == *id, None::<&str>)
        })
        .collect::<tauri::Result<Vec<_>>>()?;
    let quality_items: Vec<&dyn IsMenuItem<Wry>> = qualities.iter().map(|q| q as &dyn IsMenuItem<Wry>).collect();

    let game = Submenu::with_items(
        app,
        "&Game",
        true,
        &[
            &MenuItem::with_id(app, "reload", "&Reload game", true, Some("F5"))?,
            &Submenu::with_items(app, "Stream &quality", true, &quality_items)?,
            &CheckMenuItem::with_id(app, "hide_bars", "&Hide DMM header and footer", true, settings.hide_bars, None::<&str>)?,
            &PredefinedMenuItem::separator(app)?,
            &MenuItem::with_id(app, "exit", "E&xit", true, None::<&str>)?,
        ],
    )?;
    Menu::with_items(app, &[&game])
}

fn on_menu_event(app: &AppHandle, event: MenuEvent) {
    let id = event.id().as_ref();
    let mut settings = Settings::load(app);
    match id {
        "reload" => {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.reload();
            }
            return;
        }
        "exit" => {
            app.exit(0);
            return;
        }
        "hide_bars" => settings.hide_bars = !settings.hide_bars,
        _ => match id.strip_prefix("quality:") {
            Some(q) if q != settings.quality => settings.quality = q.to_string(),
            Some(_) => {
                // Clicking the current tier unchecked it; restore the menu.
                if let (Some(window), Ok(menu)) = (app.get_webview_window("main"), build_menu(app, &settings)) {
                    let _ = window.set_menu(menu);
                }
                return;
            }
            None => return,
        },
    }
    // Page tweaks are fixed when the webview is created, so apply the new
    // settings by restarting. The saved relay keeps the reconnect short.
    settings.save(app);
    stop_core(app);
    app.restart();
}

fn free_port() -> std::io::Result<u16> {
    Ok(TcpListener::bind("127.0.0.1:0")?.local_addr()?.port())
}

/// Starts the sidecar and follows its log: tunnel progress goes to the
/// loading screen, and the game opens once the proxy is listening (vvcore
/// only listens after the tunnel is up).
fn start_core(app: &AppHandle) -> Result<(), Box<dyn Error>> {
    stop_core(app);
    let core = app.state::<Core>();
    let generation = core.generation.load(Ordering::SeqCst);
    let (mut events, child) = app
        .shell()
        .sidecar("vvcore")?
        .args(["-listen", &format!("127.0.0.1:{}", core.port)])
        .args(state_dir_args(app))
        .spawn()?;
    *core.child.lock().unwrap() = Some(child);

    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let mut last_error = String::new();
        while let Some(event) = events.recv().await {
            if app.state::<Core>().generation.load(Ordering::SeqCst) != generation {
                return;
            }
            match event {
                CommandEvent::Stdout(line) | CommandEvent::Stderr(line) => {
                    let line = strip_ansi(&String::from_utf8_lossy(&line));
                    let line = line.trim();
                    if line.is_empty() {
                        continue;
                    }
                    println!("[vvcore] {line}");
                    if line.contains("proxy listening") {
                        open_game(&app);
                    } else if let Some(i) = line.find("tunnel: ") {
                        let _ = app.emit("core-status", &line[i + "tunnel: ".len()..]);
                    }
                    if line.contains("FATAL") || line.contains("ERROR") {
                        last_error = line.to_string();
                    }
                }
                CommandEvent::Terminated(status) => {
                    let message = if last_error.is_empty() {
                        format!("The network core stopped (exit code {:?}).", status.code)
                    } else {
                        last_error
                    };
                    show_error(&app, &message);
                    return;
                }
                CommandEvent::Error(e) => eprintln!("[vvcore] {e}"),
                _ => {}
            }
        }
    });
    Ok(())
}

/// Lets the core remember the relay that worked, so DMM keeps seeing the same
/// IP across launches.
fn state_dir_args(app: &AppHandle) -> Vec<String> {
    match app.path().app_data_dir() {
        Ok(dir) => vec!["-state-dir".into(), dir.to_string_lossy().into_owned()],
        Err(_) => Vec::new(),
    }
}

fn stop_core<R: tauri::Runtime>(app: &tauri::AppHandle<R>) {
    if let Some(core) = app.try_state::<Core>() {
        core.generation.fetch_add(1, Ordering::SeqCst);
        if let Some(child) = core.child.lock().unwrap().take() {
            let _ = child.kill();
        }
    }
}

fn open_game(app: &AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.navigate(Url::parse(GAME_URL).expect("valid game URL"));
    }
}

/// Returns to the loading screen, which shows the message from its URL
/// fragment and offers a retry.
fn show_error(app: &AppHandle, message: &str) {
    let mut url = app.state::<Core>().loading_url.clone();
    url.set_fragment(Some(&format!("error={message}")));
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.navigate(url);
    }
}

/// Removes the terminal colour codes vvcore's log handler writes.
fn strip_ansi(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    let mut chars = s.chars();
    while let Some(c) = chars.next() {
        if c == '\u{1b}' {
            for c in chars.by_ref() {
                if c.is_ascii_alphabetic() {
                    break;
                }
            }
        } else {
            out.push(c);
        }
    }
    out
}
