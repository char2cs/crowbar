mod protocol;
mod sidecar;
mod sse_bridge;

use tauri::Manager;

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_log::Builder::new().build())
        .plugin(tauri_plugin_shell::init())
        .manage(sidecar::SidecarHandle::new())
        .setup(|app| {
            let app_handle = app.handle().clone();

            // Inject window.__CROWBAR__ before the frontend loads
            if let Some(window) = app.get_webview_window("main") {
                window.eval(
                    r#"window.__CROWBAR__ = { api: 'crowbar://api', events: 'crowbar://events' };"#,
                )?;

                // Inject EventSource polyfill for crowbar://events
                let polyfill = r#"(function(){
  var _L = {};
  window.__crowbarPushEvent = function(type, data) {
    (_L[type] || []).forEach(function(fn) {
      fn(new MessageEvent(type, { data: data }));
    });
  };
  var _Orig = window.EventSource;
  function FakeES(url) {
    this.url = url; this.readyState = 1;
    this.onopen = null; this.onmessage = null; this.onerror = null;
    var self = this;
    setTimeout(function() { if (self.onopen) self.onopen(new Event('open')); }, 0);
  }
  FakeES.prototype.addEventListener = function(t, fn) {
    _L[t] = _L[t] || []; _L[t].push(fn);
  };
  FakeES.prototype.removeEventListener = function(t, fn) {
    _L[t] = (_L[t] || []).filter(function(f) { return f !== fn; });
  };
  FakeES.prototype.close = function() { this.readyState = 2; };
  FakeES.CONNECTING = 0; FakeES.OPEN = 1; FakeES.CLOSED = 2;
  window.EventSource = function(url, init) {
    if (typeof url === 'string' && url.indexOf('crowbar://events') === 0) {
      return new FakeES(url);
    }
    return new _Orig(url, init);
  };
  window.EventSource.CONNECTING = 0;
  window.EventSource.OPEN = 1;
  window.EventSource.CLOSED = 2;
})();"#;
                window.eval(polyfill)?;
            }

            // Spawn the Go daemon sidecar
            tauri::async_runtime::spawn(async move {
                if let Err(e) = sidecar::spawn(&app_handle).await {
                    log::error!("failed to start crowbar daemon: {e}");
                }
            });

            // Spawn SSE bridge (reads Go SSE stream → pushes events into webview)
            let sse_window = app
                .get_webview_window("main")
                .ok_or("main window not found")?;
            tauri::async_runtime::spawn(async move {
                sse_bridge::start(sse_window).await;
            });

            Ok(())
        })
        .register_asynchronous_uri_scheme_protocol("crowbar", |_app, request, responder| {
            tauri::async_runtime::spawn(async move {
                let response = protocol::handle(request).await;
                responder.respond(response);
            });
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { .. } | tauri::WindowEvent::Destroyed =
                event
            {
                if let Some(state) = window.try_state::<sidecar::SidecarHandle>() {
                    if let Some(child) = state.child.lock().unwrap().take() {
                        let _ = child.kill();
                    }
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("error running Tauri app");
}
