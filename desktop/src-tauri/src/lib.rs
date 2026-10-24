//! Desktop shell for Rivo: starts the Go media server as a sidecar,
//! shows its web UI in a native window, lives in the tray (so TVs keep
//! streaming when the window is closed) and hands magnet links to the server.

use std::sync::Mutex;

use tauri::menu::{CheckMenuItem, Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Manager, RunEvent, WindowEvent};
use tauri_plugin_autostart::{MacosLauncher, ManagerExt as _};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

#[derive(Default)]
struct Server {
    child: Mutex<Option<CommandChild>>,
    base: Mutex<Option<String>>,
    pending_magnet: Mutex<Option<String>>,
}

fn show_main(app: &AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.show();
        let _ = w.unminimize();
        let _ = w.set_focus();
    }
}

fn navigate(app: &AppHandle, path: &str) {
    let base = app.state::<Server>().base.lock().unwrap().clone();
    if let (Some(base), Some(w)) = (base, app.get_webview_window("main")) {
        if let Ok(u) = url::Url::parse(&format!("{base}{path}")) {
            let _ = w.navigate(u);
        }
    }
}

/// Opens the torrents page with the magnet prefilled; the person confirms
/// there (and accepts the legal notice on first use).
fn open_magnet(app: &AppHandle, magnet: &str) {
    let ready = app.state::<Server>().base.lock().unwrap().is_some();
    if !ready {
        *app.state::<Server>().pending_magnet.lock().unwrap() = Some(magnet.to_string());
        return;
    }
    let enc: String = url::form_urlencoded::byte_serialize(magnet.as_bytes()).collect();
    navigate(app, &format!("/torrents?magnet={enc}"));
    show_main(app);
}

fn magnet_in<'a, I: IntoIterator<Item = &'a str>>(args: I) -> Option<String> {
    args.into_iter().find(|a| a.starts_with("magnet:")).map(|s| s.to_string())
}

fn splash_error(app: &AppHandle, msg: &str) {
    if let Some(w) = app.get_webview_window("main") {
        let js = format!(
            "window.dispatchEvent(new CustomEvent('hc-error',{{detail:{}}}))",
            serde_json::to_string(msg).unwrap_or_default()
        );
        let _ = w.eval(&js);
    }
}

fn start_server(app: &AppHandle) -> Result<(), String> {
    let cmd = app
        .shell()
        .sidecar("homecinema-server")
        .map_err(|e| e.to_string())?
        .args(["-exit-on-stdin-close"]);
    let (mut rx, child) = cmd.spawn().map_err(|e| e.to_string())?;
    *app.state::<Server>().child.lock().unwrap() = Some(child);

    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        while let Some(ev) = rx.recv().await {
            match ev {
                CommandEvent::Stdout(line) => {
                    let line = String::from_utf8_lossy(&line);
                    if let Some(url) = line.trim().strip_prefix("HOMECINEMA_READY ") {
                        *app.state::<Server>().base.lock().unwrap() = Some(url.to_string());
                        navigate(&app, "/");
                        let pending = app.state::<Server>().pending_magnet.lock().unwrap().take();
                        if let Some(m) = pending {
                            open_magnet(&app, &m);
                        }
                    }
                }
                CommandEvent::Terminated(p) => {
                    *app.state::<Server>().base.lock().unwrap() = None;
                    splash_error(
                        &app,
                        &format!("Медиасервер остановился (код {:?}). Журнал: %APPDATA%\\HomeCinema\\logs", p.code),
                    );
                    break;
                }
                _ => {}
            }
        }
    });
    Ok(())
}

// External links (TMDB, IMDb…) open in the system browser. A link with
// target="_blank" or window.open() asks the webview for a new window, which
// the shell does not create, so the init script turns those into a plain
// navigation and the navigation hook hands any off-site URL to the browser.
const EXTERNAL_LINKS_JS: &str = r#"
(() => {
  const external = (href) => { try { const u = new URL(href, location.href); return /^https?:$/.test(u.protocol) && u.origin !== location.origin ? u.href : null; } catch { return null; } };
  document.addEventListener('click', (e) => {
    const a = e.target instanceof Element && e.target.closest('a[href]');
    const url = a && external(a.getAttribute('href'));
    if (!url || e.defaultPrevented || e.button !== 0) return;
    e.preventDefault();
    location.href = url;
  }, true);
  const open = window.open;
  window.open = (href, ...rest) => { const url = href && external(String(href)); if (url) { location.href = url; return null; } return open.call(window, href, ...rest); };
})();
"#;

fn external_links<R: tauri::Runtime>() -> tauri::plugin::TauriPlugin<R> {
    tauri::plugin::Builder::new("external-links")
        .js_init_script(EXTERNAL_LINKS_JS)
        .on_navigation(|_webview, url| {
            let local = matches!(url.host_str(), Some("127.0.0.1" | "localhost" | "tauri.localhost")) || !matches!(url.scheme(), "http" | "https");
            if !local {
                #[allow(deprecated)]
                let _ = tauri_plugin_shell::open::open(None, url.as_str(), None);
            }
            local
        })
        .build()
}

pub fn run() {
    tauri::Builder::default()
        .plugin(external_links())
        .plugin(tauri_plugin_single_instance::init(|app, argv, _cwd| {
            if let Some(m) = magnet_in(argv.iter().map(|s| s.as_str())) {
                open_magnet(app, &m);
            } else {
                show_main(app);
            }
        }))
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_autostart::init(MacosLauncher::LaunchAgent, Some(vec!["--minimized"])))
        .manage(Server::default())
        .setup(|app| {
            let handle = app.handle().clone();

            // Magnet links: first launch, and links opened while running.
            if let Ok(Some(urls)) = app.deep_link().get_current() {
                if let Some(m) = magnet_in(urls.iter().map(|u| u.as_str())) {
                    *app.state::<Server>().pending_magnet.lock().unwrap() = Some(m);
                }
            }
            let h = handle.clone();
            app.deep_link().on_open_url(move |ev| {
                if let Some(m) = magnet_in(ev.urls().iter().map(|u| u.as_str())) {
                    open_magnet(&h, &m);
                }
            });

            // Tray: the server keeps running for TVs when the window is closed.
            let open = MenuItem::with_id(app, "open", "Открыть Rivo", true, None::<&str>)?;
            let autostart_on = app.autolaunch().is_enabled().unwrap_or(false);
            let autostart = CheckMenuItem::with_id(app, "autostart", "Запускать вместе с Windows", true, autostart_on, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "Выход", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&open, &autostart, &PredefinedMenuItem::separator(app)?, &quit])?;
            let autostart_item = autostart.clone();
            TrayIconBuilder::with_id("main")
                .icon(app.default_window_icon().cloned().expect("icon"))
                .tooltip("Rivo")
                .menu(&menu)
                .show_menu_on_left_click(false)
                .on_menu_event(move |app, ev| match ev.id().as_ref() {
                    "open" => show_main(app),
                    "autostart" => {
                        let al = app.autolaunch();
                        let on = al.is_enabled().unwrap_or(false);
                        let _ = if on { al.disable() } else { al.enable() };
                        let _ = autostart_item.set_checked(al.is_enabled().unwrap_or(false));
                    }
                    "quit" => app.exit(0),
                    _ => {}
                })
                .on_tray_icon_event(|tray, ev| {
                    if let TrayIconEvent::Click { button: MouseButton::Left, button_state: MouseButtonState::Up, .. } = ev {
                        show_main(tray.app_handle());
                    }
                })
                .build(app)?;

            if std::env::args().any(|a| a == "--minimized") {
                if let Some(w) = app.get_webview_window("main") {
                    let _ = w.hide();
                }
            }

            if let Err(e) = start_server(&handle) {
                splash_error(&handle, &format!("Не удалось запустить медиасервер: {e}"));
            }
            Ok(())
        })
        .on_window_event(|window, ev| {
            if let WindowEvent::CloseRequested { api, .. } = ev {
                // Hide to tray: casting to a TV continues in the background.
                api.prevent_close();
                let _ = window.hide();
            }
        })
        .build(tauri::generate_context!())
        .expect("error while building Rivo")
        .run(|app, ev| {
            if let RunEvent::Exit = ev {
                if let Some(child) = app.state::<Server>().child.lock().unwrap().take() {
                    let _ = child.kill();
                }
            }
        });
}
