use tauri::webview::{PageLoadEvent, WebviewBuilder};
use tauri::{AppHandle, LogicalPosition, LogicalSize, Manager, Runtime, WebviewUrl};

use super::{events, find, label_for, parse_url};

fn bounds<R: Runtime>(
    webview: &tauri::Webview<R>,
    (x, y, width, height): (f64, f64, f64, f64),
) -> Result<(), String> {
    webview
        .set_bounds(tauri::Rect {
            position: LogicalPosition::new(x, y).into(),
            size: LogicalSize::new(width, height).into(),
        })
        .map_err(|e| e.to_string())
}

#[tauri::command]
#[allow(clippy::too_many_arguments)] // flat IPC args: the web side spreads the rect
pub async fn browser_open<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    url: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
    zoom: f64,
) -> Result<(), String> {
    let label = label_for(&id)?;
    let url = parse_url(&url)?;
    if let Some(existing) = app.get_webview(&label) {
        bounds(&existing, (x, y, width, height))?;
        return existing.show().map_err(|e| e.to_string());
    }
    let window = app
        .get_window("main")
        .ok_or("main window is not available")?;
    let (nav_id, load_id, title_id) = (id.clone(), id.clone(), id);
    let builder = WebviewBuilder::new(&label, WebviewUrl::External(url));
    #[cfg(target_os = "macos")]
    let builder = builder.data_store_identifier(super::data::STORE_ID);
    let builder = builder
        .on_navigation(move |_| true)
        .on_page_load(move |webview, payload| {
            let loading = matches!(payload.event(), PageLoadEvent::Started);
            events::navigated(&webview, &nav_id, payload.url().as_str());
            events::loading(&webview, &load_id, loading);
        })
        .on_document_title_changed(move |webview, title| {
            events::title(&webview, &title_id, &title);
        });
    let webview = window
        .add_child(
            builder,
            LogicalPosition::new(x, y),
            LogicalSize::new(width, height),
        )
        .map_err(|e| e.to_string())?;
    webview.set_zoom(zoom).map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_set_bounds<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
) -> Result<(), String> {
    bounds(&find(&app, &id)?, (x, y, width, height))
}

#[tauri::command]
pub async fn browser_set_visible<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    visible: bool,
) -> Result<(), String> {
    let webview = find(&app, &id)?;
    let result = if visible {
        webview.show()
    } else {
        webview.hide()
    };
    result.map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_navigate<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    url: String,
) -> Result<(), String> {
    let url = parse_url(&url)?;
    find(&app, &id)?.navigate(url).map_err(|e| e.to_string())
}

fn eval<R: Runtime>(app: &AppHandle<R>, id: &str, js: &str) -> Result<(), String> {
    find(app, id)?.eval(js).map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_back<R: Runtime>(app: AppHandle<R>, id: String) -> Result<(), String> {
    eval(&app, &id, "history.back()")
}

#[tauri::command]
pub async fn browser_forward<R: Runtime>(app: AppHandle<R>, id: String) -> Result<(), String> {
    eval(&app, &id, "history.forward()")
}

#[tauri::command]
pub async fn browser_reload<R: Runtime>(app: AppHandle<R>, id: String) -> Result<(), String> {
    find(&app, &id)?.reload().map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_close<R: Runtime>(app: AppHandle<R>, id: String) -> Result<(), String> {
    find(&app, &id)?.close().map_err(|e| e.to_string())
}
