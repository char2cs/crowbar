mod browser_pane;
mod sidecar;

use tauri::Manager;

#[cfg(target_os = "macos")]
use window_vibrancy::{apply_vibrancy, NSVisualEffectMaterial, NSVisualEffectState};

pub fn run() {
    let mut builder = tauri::Builder::default()
        .plugin(tauri_plugin_macos_fps::init())
        .plugin(tauri_plugin_log::Builder::new().build())
        .plugin(tauri_plugin_shell::init());

    // Dev-only: exposes the webview to the Tauri MCP server (WebSocket :9223).
    // Gated to debug builds so it never ships in a release.
    #[cfg(debug_assertions)]
    {
        builder = builder.plugin(tauri_plugin_mcp_bridge::init());
    }

    builder
        .manage(sidecar::SidecarHandle::new())
        .manage(browser_pane::BrowserPaneManager::new())
        .setup(|app| {
            let app_handle = app.handle().clone();

            // Apply macOS vibrancy (frosted glass) to the whole window
            if let Some(window) = app.get_webview_window("main") {
                #[cfg(target_os = "macos")]
                apply_vibrancy(
                    &window,
                    NSVisualEffectMaterial::HudWindow,
                    Some(NSVisualEffectState::FollowsWindowActiveState),
                    None,
                )
                .expect("Failed to apply vibrancy");
            }

            // The daemon listens on a reserved localhost TCP port; the webview
            // talks to it over plain HTTP + WebSocket, the same transport the
            // browser dev build uses. Inject the base URL before the frontend
            // loads so api.ts and the WS manager resolve against it.
            let port = sidecar::pick_free_port();
            if let Some(window) = app.get_webview_window("main") {
                window.eval(&format!(
                    "window.__CROWBAR__ = {{ api: 'http://127.0.0.1:{port}' }};"
                ))?;
            }

            // Spawn the Go daemon sidecar on the reserved port.
            tauri::async_runtime::spawn(async move {
                if let Err(e) = sidecar::spawn(&app_handle, port).await {
                    log::error!("failed to start crowbar daemon: {e}");
                }
            });

            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { .. } | tauri::WindowEvent::Destroyed =
                event
            {
                if let Some(state) = window.try_state::<sidecar::SidecarHandle>() {
                    if let Some(child) = state.child.lock().unwrap().take() {
                        let _ = child.kill();
                    }
                }
            }
        })
        .invoke_handler(tauri::generate_handler![
            browser_pane::browser_pane_sync,
            browser_pane::browser_pane_navigate,
            browser_pane::browser_pane_go_back,
            browser_pane::browser_pane_go_forward,
            browser_pane::browser_pane_reload,
            browser_pane::browser_pane_close,
            browser_pane::browser_pane_nav_event,
        ])
        .run(tauri::generate_context!())
        .expect("error running Tauri app");
}
