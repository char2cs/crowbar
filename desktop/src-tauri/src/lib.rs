//! The Tauri app builder: plugins, managed state, lifecycle hooks and the command table.

mod api_proxy;
#[cfg(target_os = "macos")]
mod app_menu;
#[cfg(desktop)]
mod browser;
#[cfg(desktop)]
mod console_menu;
#[cfg(desktop)]
mod context_menu;
#[cfg(debug_assertions)]
mod dev_identity;
#[cfg(desktop)]
mod diagnostics;
#[cfg(desktop)]
mod fdlimit;
#[cfg(desktop)]
mod file_manager;
#[cfg(target_os = "macos")]
mod frame_rate;
mod host_tokens;
mod hosts;
mod init_scripts;
mod logging;
mod navigation;
#[cfg(desktop)]
mod shell_plugin;
#[cfg(desktop)]
mod sidecar;
#[cfg(test)]
mod test_support;
mod tls_pin;
#[cfg(desktop)]
mod window_chrome;
#[cfg(desktop)]
mod windows;
mod ws_bridge;

use tauri::Manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // Before anything opens a descriptor: macOS starts a GUI app at a soft limit of
    // 256, and this process proxies every request and WebSocket over its own sockets.
    // No logger exists yet, so setup() reports the outcome.
    #[cfg(desktop)]
    let fd_limit = fdlimit::raise();

    #[cfg(target_os = "macos")]
    unsafe {
        frame_rate::uncap_before_webview()
    }

    #[cfg_attr(not(desktop), allow(unused_mut))] // reason: only desktop adds plugins below
    let mut builder = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(logging::plugin())
        .plugin(init_scripts::bootstrap())
        // The webview never navigates away from the app: web links open in the OS browser.
        .plugin(navigation::plugin())
        // Route webview `crowbar://localhost/v0/...` fetches to the daemon's unix socket.
        .register_asynchronous_uri_scheme_protocol("crowbar", api_proxy::handle_request);

    #[cfg(target_os = "ios")]
    {
        builder = builder.plugin(tauri_plugin_crowbar_chrome::init());
    }

    #[cfg(all(mobile, debug_assertions))]
    {
        builder = builder.plugin(init_scripts::console_forward());
    }

    #[cfg(desktop)]
    {
        // tauri-plugin-macos-fps covers macOS 13-15; frame_rate covers macOS 26.
        builder = builder
            .plugin(tauri_plugin_macos_fps::init())
            .plugin(shell_plugin::init());
    }

    // Exposes the webview to the Tauri MCP server (WebSocket :9223); never in a release.
    #[cfg(all(desktop, debug_assertions))]
    {
        builder = builder.plugin(tauri_plugin_mcp_bridge::init());
    }

    #[cfg(target_os = "macos")]
    {
        builder = builder
            .menu(app_menu::build)
            .on_menu_event(app_menu::handle_event);
    }

    #[cfg(desktop)]
    {
        builder = builder
            .manage(sidecar::SidecarHandle::new())
            .manage(browser::Parked::default());
    }

    let builder = builder
        .manage(ws_bridge::WsBridgeManager::new())
        .manage(std::sync::Arc::new(host_tokens::KeyringTokenStore::new())
            as std::sync::Arc<dyn host_tokens::TokenStore>)
        .manage(hosts::HostRegistry::default())
        // A page load orphans every connection the outgoing page owned, and nothing
        // else can notice: a `Channel` survives a reload. The daemon keeps each PTY
        // alive, so the new page re-attaches the terminals it still wants.
        .on_page_load(|webview, payload| {
            if payload.event() != tauri::webview::PageLoadEvent::Started {
                return;
            }
            let app = webview.app_handle();
            app.state::<ws_bridge::WsBridgeManager>()
                .close_for_window(webview.label());
        })
        .setup(move |app| {
            #[cfg(mobile)]
            let _ = app; // reason: only desktop has windows to decorate and a sidecar to start
            #[cfg(desktop)]
            let app_handle = app.handle().clone();

            // The first number to reach for when the app cannot dial the daemon.
            #[cfg(desktop)]
            match &fd_limit {
                fdlimit::Outcome::Failed(_) => log::warn!("{fd_limit}"),
                outcome => log::info!("{outcome}"),
            }

            // The daemon binary says where its home and socket are, before any window
            // can call in. The phone has no local daemon.
            #[cfg(desktop)]
            if !sidecar::resolve_or_explain(&app_handle) {
                return Ok(());
            }

            // `main` here; `open_window` decorates every later window.
            #[cfg(target_os = "macos")]
            if let Some(webview) = app.get_webview("main") {
                window_chrome::decorate_window(&webview);
            }

            #[cfg(all(desktop, debug_assertions))]
            dev_identity::apply_window_title(app);

            // Spawn the daemon on its unix socket, then supervise it (deep readiness
            // probes, goroutine dump + restart on a wedge).
            #[cfg(desktop)]
            {
                tauri::async_runtime::spawn(async move {
                    let Some(socket) = sidecar::socket_path() else {
                        log::error!("failed to start crowbar daemon: its paths are unresolved");
                        return;
                    };
                    if let Err(e) = sidecar::spawn(&app_handle, socket).await {
                        log::error!("failed to start crowbar daemon: {e}");
                    }
                });
                sidecar::start_watchdog(app.handle().clone());
            }

            Ok(())
        });

    #[cfg(desktop)]
    let builder = builder.on_window_event(windows::on_window_event);

    #[cfg(desktop)]
    let builder = builder.invoke_handler(tauri::generate_handler![
        ws_bridge::ws_open,
        ws_bridge::ws_send,
        ws_bridge::ws_close,
        diagnostics::diagnostics_export,
        host_tokens::forget_host_token,
        api_proxy::pairing::pair_remote_host,
        api_proxy::probe::probe_remote_fingerprint,
        hosts::register_remote_host,
        hosts::unregister_remote_host,
        file_manager::reveal_in_finder,
        window_chrome::set_vibrancy_appearance,
        window_chrome::set_traffic_light_position,
        context_menu::popup_native_context_menu,
        windows::open_window,
        console_menu::set_console_menu_chord,
        browser::browser_open,
        browser::browser_set_bounds,
        browser::browser_park,
        browser::browser_unpark,
        browser::browser_navigate,
        browser::browser_back,
        browser::browser_forward,
        browser::browser_reload,
        browser::browser_close,
        browser::browser_eval,
        browser::browser_screenshot,
        browser::browser_capabilities,
        browser::browser_clear_data,
        browser::browser_copy_text,
    ]);
    #[cfg(mobile)]
    let builder = builder.invoke_handler(tauri::generate_handler![
        ws_bridge::ws_open,
        ws_bridge::ws_send,
        ws_bridge::ws_close,
        host_tokens::forget_host_token,
        api_proxy::pairing::pair_remote_host,
        api_proxy::probe::probe_remote_fingerprint,
        hosts::register_remote_host,
        hosts::unregister_remote_host,
    ]);

    builder
        .build(tauri::generate_context!())
        .expect("error building Tauri app")
        // `build` + `run` rather than `run(context)`, for a run-event callback.
        .run(|app_handle, event| {
            #[cfg(mobile)]
            let _ = (app_handle, event); // reason: the dev icon and sidecar exit hook are desktop-only
            #[cfg(all(debug_assertions, target_os = "macos"))]
            if matches!(event, tauri::RunEvent::Ready) {
                dev_identity::apply_app_icon();
            }

            // Cmd+Q exits without closing windows one by one, so the last-window path
            // never fires; an orphaned daemon would hold the socket the next launch needs.
            #[cfg(desktop)]
            if matches!(event, tauri::RunEvent::Exit) {
                sidecar::shutdown_app(app_handle, "app exit");
            }
        });
}
