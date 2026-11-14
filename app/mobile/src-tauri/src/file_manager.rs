/// Reveals a path in the OS file manager. An app command, so no opener-plugin path scope
/// is needed (a future `permissions/` dir must allow it). `spawn_blocking`: Finder's XPC
/// round trip would otherwise block the main thread, and with it the whole webview.
#[tauri::command]
pub async fn reveal_in_finder(path: String) -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(move || {
        tauri_plugin_opener::reveal_item_in_dir(&path).map_err(|e| e.to_string())
    })
    .await
    .map_err(|e| format!("reveal_in_finder task panicked: {e}"))?
}

#[cfg(test)]
mod tests {
    use super::reveal_in_finder;

    /// A nonexistent path fails at canonicalize, before any Finder/XPC call, so this
    /// exercises only the async + spawn_blocking plumbing propagating the error.
    #[tokio::test]
    async fn reveal_in_finder_is_async_and_propagates_errors() {
        let result = reveal_in_finder("/definitely/does/not/exist/crowbar-task-30".into()).await;
        assert!(
            result.is_err(),
            "canonicalize on a nonexistent path must fail"
        );
    }
}
