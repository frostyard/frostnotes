//! The Frostnotes app shell: one instance with one main window and a line
//! bridge to notesd's socket. All notes logic lives in notesd; see
//! docs/design/overview.md and docs/specs/rpc-protocol.md.

mod bridge;
mod paths;
mod spike;

use tauri::webview::NewWindowResponse;
use tauri::{Manager, WebviewUrl, WebviewWindowBuilder};

fn main() {
    apply_webkit_workarounds();
    let spike = spike::Spike::from_env();
    let spiking = spike.active();
    let timed = spiking && !spike.manual();
    let mut builder = tauri::Builder::default();
    // A spike run measures its own process, so it never hands over to an
    // app already running.
    if !spiking {
        // First, so a second launch hands over before anything else starts.
        builder = builder.plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| second_launch(app)));
    }
    builder
        .manage(bridge::Bridge::default())
        .manage(spike)
        .invoke_handler(tauri::generate_handler![
            bridge::notesd_connect,
            bridge::notesd_send,
            spike::spike_mode,
            spike::spike_report
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                let app = window.app_handle().clone();
                let label = window.label().to_string();
                tauri::async_runtime::spawn(async move { app.state::<bridge::Bridge>().forget(&label).await });
            }
        })
        .setup(move |app| {
            let main = WebviewWindowBuilder::new(app, "main", WebviewUrl::default())
                .title("Frostnotes")
                .inner_size(1200.0, 780.0)
                .min_inner_size(640.0, 400.0)
                // The title bar is the page's own (ADR-0011).
                .decorations(false)
                .on_navigation(allowed_navigation)
                .on_new_window(|_, _| NewWindowResponse::Deny)
                .visible(false)
                .build()?;
            show_soon(main);
            if timed {
                spike::watchdog(app.handle().clone());
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("frostnotes failed to start");
}

/// A second launch (single instance) shows the main window.
fn second_launch(app: &tauri::AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
    }
}

/// The window starts hidden and the page shows it once it has rendered, so
/// the first frame is never WebKit's blank white page; if the page has not
/// done so after 1.5 s, the window is shown anyway.
fn show_soon(window: tauri::WebviewWindow) {
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(std::time::Duration::from_millis(1500)).await;
        if window.is_visible().is_ok_and(|v| !v) {
            let _ = window.show();
        }
    });
}

/// Only the app itself may load in the main frame.
fn allowed_navigation(url: &tauri::Url) -> bool {
    match url.scheme() {
        "tauri" | "about" => true,
        "http" => cfg!(debug_assertions) && url.host_str() == Some("127.0.0.1") && url.port() == Some(5173),
        _ => false,
    }
}

/// WebKitGTK 2.52+ has GPU rendering bugs on some AMD (radv) and NVIDIA
/// setups (frostmail's ADR-0006). FROSTNOTES_WEBKIT_SAFE=1 selects the CPU
/// paths unless the variables are already set.
fn apply_webkit_workarounds() {
    if std::env::var_os("FROSTNOTES_WEBKIT_SAFE").is_none_or(|v| v != "1") {
        return;
    }
    for key in ["WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_SKIA_ENABLE_CPU_RENDERING"] {
        if std::env::var_os(key).is_none() {
            // SAFETY: called first in main, before any other thread exists.
            unsafe { std::env::set_var(key, "1") };
        }
    }
}

#[cfg(test)]
mod tests {
    use super::allowed_navigation;

    #[test]
    fn navigates_only_within_the_app() {
        for url in ["tauri://localhost/", "about:blank"] {
            assert!(allowed_navigation(&tauri::Url::parse(url).unwrap()), "{url}");
        }
        for url in ["https://example.test/", "file:///etc/passwd", "javascript:alert(1)", "http://127.0.0.1:8080/"] {
            assert!(!allowed_navigation(&tauri::Url::parse(url).unwrap()), "{url}");
        }
        let dev = tauri::Url::parse("http://127.0.0.1:5173/").unwrap();
        assert_eq!(allowed_navigation(&dev), cfg!(debug_assertions));
    }
}
