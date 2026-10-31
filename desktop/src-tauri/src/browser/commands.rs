use tauri::webview::{PageLoadEvent, WebviewBuilder};
use tauri::{AppHandle, LogicalPosition, LogicalSize, Manager, Runtime, State, WebviewUrl};

use super::park::{self, Parked};
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

/// Moves the view out of the window at its current size (see `park`).
fn move_away<R: Runtime>(webview: &tauri::Webview<R>) -> Result<(), String> {
    let scale = webview.window().scale_factor().map_err(|e| e.to_string())?;
    let width = webview.size().map_err(|e| e.to_string())?.width as f64 / scale;
    let (x, y) = park::origin(width);
    webview
        .set_position(LogicalPosition::new(x, y))
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
    parked: bool,
    registry: State<'_, Parked>,
) -> Result<(), String> {
    let label = label_for(&id)?;
    let url = parse_url(&url)?;
    if let Some(existing) = app.get_webview(&label) {
        return if parked {
            park_view(&existing, &registry)
        } else {
            unpark_view(&existing, &registry, (x, y, width, height))
        };
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
    let (x, y) = if parked {
        registry.park(&label);
        park::origin(width)
    } else {
        (x, y)
    };
    let webview = window
        .add_child(
            builder,
            LogicalPosition::new(x, y),
            LogicalSize::new(width, height),
        )
        .inspect_err(|_| registry.unpark(&label))
        .map_err(|e| e.to_string())?;
    webview.set_zoom(zoom).map_err(|e| e.to_string())
}

fn park_view<R: Runtime>(webview: &tauri::Webview<R>, registry: &Parked) -> Result<(), String> {
    let newly_parked = !registry.is_parked(webview.label());
    registry.park(webview.label());
    move_away(webview)?;
    // An off-window view stays first responder unless it is handed back.
    if newly_parked {
        if let Some(main) = webview.app_handle().get_webview("main") {
            main.set_focus().map_err(|e| e.to_string())?;
        }
    }
    Ok(())
}

fn unpark_view<R: Runtime>(
    webview: &tauri::Webview<R>,
    registry: &Parked,
    frame: (f64, f64, f64, f64),
) -> Result<(), String> {
    registry.unpark(webview.label());
    bounds(webview, frame)
}

#[tauri::command]
pub async fn browser_set_bounds<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
    registry: State<'_, Parked>,
) -> Result<(), String> {
    let webview = find(&app, &id)?;
    if registry.is_parked(webview.label()) {
        return Ok(());
    }
    bounds(&webview, (x, y, width, height))
}

#[tauri::command]
pub async fn browser_park<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    registry: State<'_, Parked>,
) -> Result<(), String> {
    park_view(&find(&app, &id)?, &registry)
}

#[tauri::command]
pub async fn browser_unpark<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
    registry: State<'_, Parked>,
) -> Result<(), String> {
    unpark_view(&find(&app, &id)?, &registry, (x, y, width, height))
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
pub async fn browser_close<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    registry: State<'_, Parked>,
) -> Result<(), String> {
    let webview = find(&app, &id)?;
    registry.unpark(webview.label());
    webview.close().map_err(|e| e.to_string())
}
