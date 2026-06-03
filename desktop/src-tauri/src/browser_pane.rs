use std::collections::HashMap;
use std::sync::Mutex;

use tauri::{AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, State, WebviewUrl};
use tauri::webview::WebviewBuilder;

const NAV_SCRIPT_TEMPLATE: &str = r#"
(function() {
  var B = '__CROWBAR_BID__';
  var sess = sessionStorage;
  var idx = parseInt(sess.getItem('__ci_' + B) || '0');
  var max = parseInt(sess.getItem('__cm_' + B) || '0');

  function save() {
    sess.setItem('__ci_' + B, String(idx));
    sess.setItem('__cm_' + B, String(max));
  }

  function emit(url) {
    if (!window.__TAURI_INTERNALS__) return;
    window.__TAURI_INTERNALS__.invoke('browser_pane_nav_event', {
      bufferId: B,
      url: url,
      canGoBack: idx > 0,
      canGoForward: idx < max,
    }).catch(function(){});
  }

  var origPush = history.pushState.bind(history);
  history.pushState = function(state, title, url) {
    idx += 1;
    max = Math.max(max, idx);
    origPush(Object.assign({}, typeof state === 'object' ? state : {}, { __ci: idx, __cm: max }), title, url);
    save();
    emit(String(url || location.href));
  };

  var origReplace = history.replaceState.bind(history);
  history.replaceState = function(state, title, url) {
    origReplace(Object.assign({}, typeof state === 'object' ? state : {}, { __ci: idx, __cm: max }), title, url);
    save();
    emit(String(url || location.href));
  };

  window.addEventListener('popstate', function(e) {
    var s = history.state;
    if (s && typeof s.__ci === 'number') {
      idx = s.__ci;
      max = Math.max(max, typeof s.__cm === 'number' ? s.__cm : idx);
    } else {
      idx = Math.max(0, idx - 1);
    }
    save();
    emit(location.href);
  });

  window.addEventListener('load', function() {
    emit(location.href);
  });
})();
"#;

pub struct BrowserPaneManager {
    panes: Mutex<HashMap<String, tauri::Webview>>,
}

impl BrowserPaneManager {
    pub fn new() -> Self {
        Self {
            panes: Mutex::new(HashMap::new()),
        }
    }
}

fn make_init_script(buffer_id: &str) -> String {
    NAV_SCRIPT_TEMPLATE.replace("__CROWBAR_BID__", buffer_id)
}

#[tauri::command]
pub async fn browser_pane_sync(
    app: AppHandle,
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
    visible: bool,
    // Used only on creation — the webview starts at this URL so no
    // separate browser_pane_navigate call is needed on mount.
    initial_url: Option<String>,
) -> Result<(), String> {
    let mut panes = state.panes.lock().map_err(|e| e.to_string())?;

    if let Some(webview) = panes.get(&buffer_id) {
        webview
            .set_bounds(tauri::Rect {
                position: tauri::Position::Logical(LogicalPosition::new(x, y)),
                size: tauri::Size::Logical(LogicalSize::new(width, height)),
            })
            .map_err(|e| e.to_string())?;
        if visible {
            webview.show().map_err(|e| e.to_string())?;
        } else {
            webview.hide().map_err(|e| e.to_string())?;
        }
    } else {
        let main_window = app
            .get_window("main")
            .ok_or_else(|| "main window not found".to_string())?;

        let label = format!("browser-pane-{}", buffer_id);
        let init_script = make_init_script(&buffer_id);

        let start_url = initial_url
            .filter(|u| !u.is_empty() && u != "about:blank")
            .and_then(|u| u.parse().ok())
            .unwrap_or_else(|| "about:blank".parse().unwrap());

        let webview = main_window
            .add_child(
                WebviewBuilder::new(&label, WebviewUrl::External(start_url))
                    .initialization_script(&init_script),
                LogicalPosition::new(x, y),
                LogicalSize::new(width, height),
            )
            .map_err(|e| e.to_string())?;

        if !visible {
            webview.hide().map_err(|e| e.to_string())?;
        }

        panes.insert(buffer_id, webview);
    }

    Ok(())
}

#[tauri::command]
pub async fn browser_pane_navigate(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
    url: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes
        .get(&buffer_id)
        .ok_or_else(|| format!("no pane for {buffer_id}"))?;
    // Escape single quotes to prevent JS injection via URL
    let safe_url = url.replace('\'', "%27");
    webview
        .eval(&format!("location.href = '{safe_url}'"))
        .map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_go_back(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes
        .get(&buffer_id)
        .ok_or_else(|| format!("no pane for {buffer_id}"))?;
    webview.eval("history.back()").map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_go_forward(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes
        .get(&buffer_id)
        .ok_or_else(|| format!("no pane for {buffer_id}"))?;
    webview.eval("history.forward()").map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_reload(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes
        .get(&buffer_id)
        .ok_or_else(|| format!("no pane for {buffer_id}"))?;
    webview.eval("location.reload()").map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_close(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let mut panes = state.panes.lock().map_err(|e| e.to_string())?;
    if let Some(webview) = panes.remove(&buffer_id) {
        webview.close().map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
pub async fn browser_pane_nav_event(
    app: AppHandle,
    buffer_id: String,
    url: String,
    can_go_back: bool,
    can_go_forward: bool,
) -> Result<(), String> {
    app.emit_to(
        tauri::EventTarget::WebviewWindow { label: "main".into() },
        "browser-pane-navigated",
        serde_json::json!({
            "bufferId": buffer_id,
            "url": url,
            "canGoBack": can_go_back,
            "canGoForward": can_go_forward,
        }),
    )
    .map_err(|e| e.to_string())
}
