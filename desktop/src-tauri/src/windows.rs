//! Multi-window lifecycle: opening another Crowbar window, and what a closing window
//! retires — its own transports always, the app-wide daemon only after the last one.

use tauri::{Manager, WindowEvent};

use crate::window_chrome::decorate_window;
use crate::{sidecar, ws_bridge};

/// The init script that seeds `location.hash` to `route` on a window's first paint.
/// `route` is JSON-encoded so a `"` or `\` cannot break out of the JS literal; the
/// guard ignores `'#/'`, a real route a reload must keep, not "nothing loaded yet".
fn seed_hash_script(route: &str) -> Result<String, String> {
    let encoded = serde_json::to_string(route).map_err(|e| format!("encode route: {e}"))?;
    Ok(format!(
        "if (!location.hash) {{ location.hash = {encoded}; }}"
    ))
}

/// Opens another window at `route` (a hash-router path, `/` for the picker).
/// Built `from_config` off `app.windows[0]` so it matches `main` exactly (a hand-made
/// builder misplaced the traffic lights); `async` because building a window from a
/// sync command deadlocks on Windows.
#[tauri::command]
pub async fn open_window(app: tauri::AppHandle, route: String) -> Result<(), String> {
    use std::sync::atomic::{AtomicU64, Ordering};
    static NEXT_WINDOW: AtomicU64 = AtomicU64::new(2);

    let label = format!("w{}", NEXT_WINDOW.fetch_add(1, Ordering::Relaxed));
    let seed_hash = seed_hash_script(&route)?;

    // The config entry also describes `main`, and two live windows cannot share a label.
    let mut config = tauri::Manager::config(&app)
        .app
        .windows
        .first()
        .cloned()
        .ok_or_else(|| {
            log::error!("open_window: tauri.conf.json declares no windows");
            "no window config to clone".to_string()
        })?;
    config.label = label;

    let window = tauri::WebviewWindowBuilder::from_config(&app, &config)
        .map_err(|e| {
            log::error!("open_window: config is not a valid window: {e}");
            format!("open window: {e}")
        })?
        .initialization_script(&seed_hash)
        .build()
        .map_err(|e| {
            log::error!("open_window: build failed: {e}");
            format!("open window: {e}")
        })?;

    // `apply_vibrancy` refuses to run off the main thread, and this command runs on
    // tokio; without the hop the translucent chrome shows the raw desktop through it.
    let decorated = window.clone();
    window
        .run_on_main_thread(move || decorate_window(decorated.as_ref()))
        .map_err(|e| {
            log::error!("open_window: could not decorate on the main thread: {e}");
            format!("decorate window: {e}")
        })?;
    Ok(())
}

/// Window-level labels: `webview_windows()` omits a window once it hosts a child
/// webview (a browser tab), which would make the last-window check see none.
fn live_window_labels<R: tauri::Runtime>(app: &tauri::AppHandle<R>) -> Vec<String> {
    tauri::Manager::windows(app).into_keys().collect()
}

/// Does any window other than `closing` still exist? A length threshold is wrong:
/// tauri still lists the closing window on `CloseRequested` but has already pruned
/// it on `Destroyed`, and both events reach the close handler.
fn another_window_survives<'a>(labels: impl Iterator<Item = &'a str>, closing: &str) -> bool {
    labels.into_iter().any(|label| label != closing)
}

/// Retires a closing window's transports (its JS will never call `ws_close`, and it
/// will never load a page again), then stops the daemon once the last window is gone.
pub fn on_window_event(window: &tauri::Window, event: &WindowEvent) {
    if !matches!(
        event,
        WindowEvent::CloseRequested { .. } | WindowEvent::Destroyed
    ) {
        return;
    }
    let label = window.label();

    // Idempotent, so the second pass for the same window (CloseRequested then
    // Destroyed) is a no-op.
    let app = window.app_handle();
    app.state::<ws_bridge::WsBridgeManager>()
        .close_for_window(label);

    // Only on Destroyed: a CloseRequested can still be prevented, and stopping the
    // daemon under a window that stays open strands it.
    if !matches!(event, WindowEvent::Destroyed) {
        return;
    }
    let windows = live_window_labels(app);
    if another_window_survives(windows.iter().map(String::as_str), label) {
        return;
    }

    sidecar::shutdown_app(app, "last window closed");
}

#[cfg(test)]
mod tests {
    use super::{another_window_survives, live_window_labels, seed_hash_script};
    use tauri::test::mock_app;
    use tauri::webview::WebviewBuilder;
    use tauri::{LogicalPosition, LogicalSize, WebviewUrl, WebviewWindowBuilder};

    /// A window hosting a browser child webview is no longer a "webview window"
    /// to tauri, so `webview_windows()` drops it; it must still count as alive.
    #[test]
    fn a_window_hosting_a_browser_child_still_counts_as_a_window() {
        let app = mock_app();
        let main = WebviewWindowBuilder::new(&app, "main", WebviewUrl::default())
            .build()
            .unwrap();
        main.as_ref()
            .window()
            .add_child(
                WebviewBuilder::new("browser-a", WebviewUrl::default()),
                LogicalPosition::new(0.0, 0.0),
                LogicalSize::new(10.0, 10.0),
            )
            .unwrap();

        let labels = live_window_labels(app.handle());
        assert_eq!(labels, ["main"]);
        assert!(another_window_survives(
            labels.iter().map(String::as_str),
            "w2"
        ));
    }

    /// The same close arrives as CloseRequested (still listed) and Destroyed (already
    /// pruned); a `len() > 1` guard gets the second wrong and kills the daemon.
    #[test]
    fn a_non_last_window_closing_leaves_the_daemon_alone_in_both_event_shapes() {
        assert!(
            another_window_survives(["main", "w2"].into_iter(), "w2"),
            "CloseRequested: main survives w2, so the daemon must stay up"
        );
        assert!(
            another_window_survives(["main"].into_iter(), "w2"),
            "Destroyed: main survives w2, so the daemon must stay up"
        );
    }

    /// The daemon must come down with the last window, or the next launch cannot bind.
    #[test]
    fn the_last_window_closing_takes_the_daemon_down_in_both_event_shapes() {
        assert!(
            !another_window_survives(["main"].into_iter(), "main"),
            "CloseRequested: nothing survives main, so the daemon must stop"
        );
        assert!(
            !another_window_survives(std::iter::empty(), "main"),
            "Destroyed: nothing survives main, so the daemon must stop"
        );
    }

    #[test]
    fn several_surviving_windows_all_keep_the_daemon_up() {
        assert!(another_window_survives(
            ["main", "w2", "w3"].into_iter(),
            "w2"
        ));
        assert!(another_window_survives(["main", "w3"].into_iter(), "w2"));
    }

    /// The guard fires only on an empty hash, never on `'#/'`: matching the root route
    /// would re-seed on every reload after the user navigates back to the picker.
    #[test]
    fn guard_fires_only_on_genuinely_empty_hash() {
        let script = seed_hash_script("/ide/p/r/w").expect("encode should succeed");
        assert_eq!(
            script,
            "if (!location.hash) { location.hash = \"/ide/p/r/w\"; }"
        );
        assert!(
            !script.contains("'#/'") && !script.contains("\"#/\""),
            "guard must not special-case the root route: {script}"
        );
    }

    /// "/" is what the New Window menu item passes.
    #[test]
    fn the_root_route_is_seeded_like_any_other() {
        let script = seed_hash_script("/").expect("encode should succeed");
        assert_eq!(script, "if (!location.hash) { location.hash = \"/\"; }");
    }

    /// A route is routing state, so it is untrusted input to the JS literal.
    #[test]
    fn special_characters_round_trip_into_a_valid_js_string_literal() {
        let route = "/ide/\"quote\"/back\\slash/new\nline";
        let script = seed_hash_script(route).expect("encode should succeed");

        let prefix = "if (!location.hash) { location.hash = ";
        let suffix = "; }";
        assert!(
            script.starts_with(prefix) && script.ends_with(suffix),
            "unexpected script shape: {script}"
        );
        let literal = &script[prefix.len()..script.len() - suffix.len()];

        // A raw newline inside a single-line JS string literal is a syntax error.
        assert!(
            !literal.contains('\n'),
            "raw newline leaked into the script: {script}"
        );

        // JSON and JS string-literal escaping coincide for `"`, `\` and control
        // characters, so decoding as JSON recovers what a JS engine would parse.
        let decoded: String =
            serde_json::from_str(literal).expect("literal must be valid JSON/JS string syntax");
        assert_eq!(decoded, route);
    }
}
