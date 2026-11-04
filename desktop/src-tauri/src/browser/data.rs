//! Browsing data of the browser tabs. Clearing is only offered where the tabs
//! have a website data store of their own: elsewhere it would also wipe the
//! app's own storage, since the app webview shares the default store.
use tauri::{AppHandle, Manager, Runtime};

use super::id_from_label;

/// Fixed identifier of the browser tabs' website data store (macOS 14+).
#[cfg(target_os = "macos")]
pub const STORE_ID: [u8; 16] = *b"crowbar.browser\0";

#[cfg(target_os = "macos")]
fn has_own_store() -> bool {
    use objc2_foundation::{NSOperatingSystemVersion, NSProcessInfo};
    NSProcessInfo::processInfo().isOperatingSystemAtLeastVersion(NSOperatingSystemVersion {
        majorVersion: 14,
        minorVersion: 0,
        patchVersion: 0,
    })
}

#[cfg(not(target_os = "macos"))]
fn has_own_store() -> bool {
    false
}

#[tauri::command]
pub fn browser_capabilities() -> serde_json::Value {
    serde_json::json!({ "clearData": has_own_store() })
}

#[tauri::command]
pub async fn browser_clear_data<R: Runtime>(app: AppHandle<R>) -> Result<(), String> {
    if !has_own_store() {
        return Err(
            "browsing data cannot be cleared separately from the app on this system".into(),
        );
    }
    let webview = app
        .webviews()
        .into_iter()
        .find(|(label, _)| id_from_label(label).is_some())
        .map(|(_, webview)| webview)
        .ok_or("open a browser tab to clear its data")?;
    webview.clear_all_browsing_data().map_err(|e| e.to_string())
}
