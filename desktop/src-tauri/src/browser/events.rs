use serde_json::json;
use tauri::{Emitter, Runtime, Webview};

const TARGET: &str = "main";

fn emit<R: Runtime>(webview: &Webview<R>, event: &str, payload: serde_json::Value) {
    if let Err(e) = webview.emit_to(TARGET, event, payload) {
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
