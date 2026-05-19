mod protocol;
mod sidecar;

use tauri::Manager;

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_log::Builder::new().build())
        .plugin(tauri_plugin_shell::init())
        .manage(sidecar::SidecarHandle::new())
        .setup(|app| {
            let app_handle = app.handle().clone();

            // Inject window.__CROWBAR__ before the frontend loads
            if let Some(window) = app.get_webview_window("main") {
                window.eval(
                    r#"window.__CROWBAR__ = { api: 'crowbar://api', events: 'crowbar://events' };"#,
                )?;
            }

            // Spawn the Go daemon sidecar
            tauri::async_runtime::spawn(async move {
                if let Err(e) = sidecar::spawn(&app_handle).await {
                    log::error!("failed to start crowbar daemon: {e}");
                }
            });

            Ok(())
        })
        .register_asynchronous_uri_scheme_protocol("crowbar", |_app, request, responder| {
            tauri::async_runtime::spawn(async move {
                let response = protocol::handle(request).await;
                responder.respond(response);
            });
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
        .run(tauri::generate_context!())
        .expect("error running Tauri app");
}
