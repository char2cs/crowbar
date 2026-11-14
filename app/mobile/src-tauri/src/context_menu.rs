use tauri::menu::{ContextMenu, Menu};
use tauri::Manager;

/// Pops a native context menu built via `@tauri-apps/api/menu`'s `Menu.new()` at
/// (`x`, `y`). Replaces tauri's `plugin:menu|popup`, which holds the webview's resource
/// table lock across the blocking popup and so wedges every resource-backed command.
#[tauri::command]
pub async fn popup_native_context_menu(
    webview: tauri::Webview,
    window: tauri::Window,
    rid: tauri::ResourceId,
    x: f64,
    y: f64,
) -> Result<(), String> {
    let menu: std::sync::Arc<Menu<tauri::Wry>> = {
        let table = webview.resources_table();
        table
            .get::<Menu<tauri::Wry>>(rid)
            .map_err(|e| e.to_string())?
        // The table guard drops here, before the blocking popup below.
    };

    // Off the main thread: `popup_at` dispatches to main and would deadlock from there.
    tauri::async_runtime::spawn_blocking(move || {
        menu.popup_at(window, tauri::LogicalPosition::new(x, y))
            .map_err(|e| e.to_string())
    })
    .await
    .map_err(|e| format!("popup_native_context_menu task panicked: {e}"))?
}
