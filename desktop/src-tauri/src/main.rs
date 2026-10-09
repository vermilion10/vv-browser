// Prevents an extra console window on Windows in release builds.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::error::Error;
use std::net::TcpListener;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Mutex;

use tauri::{AppHandle, Emitter, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;
use url::Url;

const GAME_URL: &str = "https://play-cloud.games.dmm.com/cloudgame/gameplay/doaxvv";

/// DMM embeds the Ubitus player without a quality, so it defaults to "mid"
/// (2 Mbps). Reload the player frame asking for "high" (6-8 Mbps).
const HIGH_QUALITY_JS: &str = r#"
if (location.hostname === 'dcgp-game.ugamenow.com' &&
    location.pathname.startsWith('/gungnir/') && location.search &&
    !/[?&]profile\.quality=/.test(location.search)) {
    location.replace(location.href + '&profile.quality=high');
}
"#;

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
        .setup(|app| {
            let port = free_port()?;
            let window = WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
                .title("VV Browser")
                .inner_size(1280.0, 800.0)
                .min_inner_size(640.0, 400.0)
                .proxy_url(Url::parse(&format!("http://127.0.0.1:{port}"))?)
                .initialization_script_for_all_frames(HIGH_QUALITY_JS)
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
