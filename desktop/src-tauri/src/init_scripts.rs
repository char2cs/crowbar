//! Scripts injected at document-start on every page load, before any frontend JS.

use tauri::plugin::TauriPlugin;

/// Sets the API base the frontend resolves against (`api.ts` / `ws/url.ts`). An init
/// script, not a one-time eval, because a reload wipes `window.__CROWBAR__`; guarded by
/// hostname so browser-pane webviews on external sites never see the global.
const CROWBAR_BOOTSTRAP: &str = r#"
(function () {
  var h = location.hostname;
  if (h === 'localhost' || h === 'tauri.localhost') {
    window.__CROWBAR__ = Object.assign(window.__CROWBAR__ || {}, {
      mode: 'local',
      endpoint: 'crowbar://localhost',
      api: 'crowbar://localhost',
    });
  }
})();
"#;

/// Forwards `console.warn/error`, uncaught errors and rejected promises to the log plugin.
#[cfg(all(mobile, debug_assertions))]
const WEBVIEW_CONSOLE_FORWARD: &str = r#"
(function () {
  function send(level, parts) {
    try {
      var message = parts.map(function (p) {
        return p instanceof Error ? p.stack || String(p) : typeof p === 'string' ? p : JSON.stringify(p);
      }).join(' ');
      window.__TAURI_INTERNALS__.invoke('plugin:log|log', { level: level, message: message, location: 'webview' });
    } catch (_) {}
  }
  ['warn', 'error'].forEach(function (name) {
    var original = console[name];
    console[name] = function () {
      send(name === 'error' ? 5 : 4, Array.prototype.slice.call(arguments));
      original.apply(console, arguments);
    };
  });
  window.addEventListener('error', function (e) { send(5, ['uncaught', e.message, e.filename + ':' + e.lineno]); });
  window.addEventListener('unhandledrejection', function (e) { send(5, ['unhandledrejection', e.reason]); });
})();
"#;

/// Injects the desktop API endpoint on every webview load.
pub fn bootstrap() -> TauriPlugin<tauri::Wry> {
    tauri::plugin::Builder::<tauri::Wry, ()>::new("crowbar-bootstrap")
        .js_init_script(CROWBAR_BOOTSTRAP.to_string())
        .build()
}

/// A phone's WKWebView has no MCP bridge, so page errors go to the native log instead.
#[cfg(all(mobile, debug_assertions))]
pub fn console_forward() -> TauriPlugin<tauri::Wry> {
    tauri::plugin::Builder::<tauri::Wry, ()>::new("webview-console")
        .js_init_script(WEBVIEW_CONSOLE_FORWARD.to_string())
        .build()
}
