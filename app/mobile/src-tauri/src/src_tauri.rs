use std::sync::Arc;

use tauri::webview::{PageLoadEvent, PageLoadPayload};
use tauri::{Builder, Manager, Runtime, Webview};

use crate::host_tokens::{KeyringTokenStore, TokenStore};
use crate::hosts::HostRegistry;
use crate::ws_bridge::WsBridgeManager;
use crate::{api_proxy, host_commands, host_tokens, init_scripts, logging, navigation, ws_bridge};

/// Starts the app; on a build failure it logs the cause and aborts, as a startup panic would.
#[cfg_attr(mobile, tauri::mobile_entry_point)]
#[expect(
    clippy::disallowed_methods,
    reason = "tauri::generate_context! expands to a process exit on a corrupt context"
)]
pub fn run() {
    match assemble::<tauri::Wry>().build(tauri::generate_context!()) {
        Ok(app) => app.run(|_, _| {}),
        Err(error) => {
            log::error!("error building Tauri app: {error}");
            std::process::abort()
        },
    }
}

fn assemble<R: Runtime>() -> Builder<R> {
    base_builder()
        .manage(WsBridgeManager::default())
        .manage(Arc::new(KeyringTokenStore::default()) as Arc<dyn TokenStore>)
        .manage(HostRegistry::default())
        .on_page_load(close_orphaned_connections)
        .invoke_handler(tauri::generate_handler![
            ws_bridge::ws_open,
            ws_bridge::ws_send,
            ws_bridge::ws_close,
            host_tokens::forget_host_token,
            api_proxy::pairing::pair_remote_host,
            api_proxy::probe::probe_remote_fingerprint,
            host_commands::register_remote_host,
            host_commands::unregister_remote_host,
        ])
}

fn base_builder<R: Runtime>() -> Builder<R> {
    let builder = Builder::new()
        .plugin(tauri_plugin_dialog::init())
        .plugin(logging::plugin())
        .plugin(init_scripts::bootstrap())
        .plugin(navigation::plugin())
        .plugin(tauri_plugin_crowbar_chrome::init())
        .register_asynchronous_uri_scheme_protocol("crowbar", api_proxy::handle_request);
    with_console_forward(builder)
}

#[cfg(debug_assertions)]
fn with_console_forward<R: Runtime>(builder: Builder<R>) -> Builder<R> {
    builder.plugin(init_scripts::console_forward())
}

#[cfg(not(debug_assertions))]
fn with_console_forward<R: Runtime>(builder: Builder<R>) -> Builder<R> {
    builder
}

fn close_orphaned_connections<R: Runtime>(
    webview: &Webview<R>,
    payload: &PageLoadPayload<'_>,
) {
    if payload.event() != PageLoadEvent::Started {
        return;
    }
    webview
        .app_handle()
        .state::<WsBridgeManager>()
        .close_for_window(webview.label());
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri::test::{mock_context, noop_assets, MockRuntime};

    #[test]
    fn the_app_assembles_with_every_plugin_and_command_registered() {
        let built = assemble::<MockRuntime>().build(mock_context(noop_assets()));
        assert!(built.is_ok(), "assembly failed: {:?}", built.err());
    }
}
