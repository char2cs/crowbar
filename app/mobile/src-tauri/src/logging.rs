use log::LevelFilter;
use tauri::plugin::TauriPlugin;

/// Info by default (the plugin's Trace drowns the log in the WebSocket stack's
/// per-frame internals), with the chattiest dependencies capped at Warn so a later
/// bump for our own code does not reopen the firehose.
pub fn plugin() -> TauriPlugin<tauri::Wry> {
    tauri_plugin_log::Builder::new()
        .level(LevelFilter::Info)
        .level_for("tokio_tungstenite", LevelFilter::Warn)
        // Never below Warn: its trace of a remote upgrade holds the bearer token.
        .level_for("tungstenite", LevelFilter::Warn)
        .level_for("hyper", LevelFilter::Warn)
        .level_for("hyper_util", LevelFilter::Warn)
        .level_for("reqwest", LevelFilter::Warn)
        .level_for("mio", LevelFilter::Warn)
        .build()
}
