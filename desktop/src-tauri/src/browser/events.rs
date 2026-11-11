use serde_json::json;
use tauri::{AppHandle, Emitter, Runtime, Webview};

const TARGET: &str = "main";

fn emit<R: Runtime>(from: &impl Emitter<R>, event: &str, payload: serde_json::Value) {
    if let Err(e) = from.emit_to(TARGET, event, payload) {
        log::warn!("browser: emit {event} failed: {e}");
    }
}

pub fn navigated<R: Runtime>(webview: &Webview<R>, id: &str, url: &str) {
    emit(
        webview,
        "browser://navigated",
        json!({ "id": id, "url": url }),
    );
}

pub fn title<R: Runtime>(webview: &Webview<R>, id: &str, title: &str) {
    emit(
        webview,
        "browser://title",
        json!({ "id": id, "title": title }),
    );
}

pub fn loading<R: Runtime>(webview: &Webview<R>, id: &str, loading: bool) {
    emit(
        webview,
        "browser://loading",
        json!({ "id": id, "loading": loading }),
    );
}

/// The pane opens it as a tab: a native window would float outside the app, untracked.
pub fn new_window<R: Runtime>(app: &AppHandle<R>, id: &str, url: &str) {
    emit(app, "browser://new-window", json!({ "id": id, "url": url }));
}
