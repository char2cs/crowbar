mod api_proxy;
mod browser_pane;
mod browser_proxy;
mod sidecar;
mod terminal;

use tauri::Manager;

// ProMotion / high-refresh-rate: WKWebView defaults to 60fps due to the
// `preferPageRenderingUpdatesNear60FPSEnabled` WebKit preference, and macOS
// ProMotion adaptively drops below 120fps when content is static.
//
// Three-pronged approach:
//   1. NSUserDefaults before WKWebView creation (disable_webkit_60fps_cap_early).
//      Works on macOS 13–15 where WebKit reads this key before creating its
//      CADisplayLink. May be ignored on macOS 26 where the preference backend
//      was restructured.
//   2. Direct WKPreferences setter to remove the 60fps cap.
//      Guarded by respondsToSelector: — safe in FFI context.
//   3. preferredFrameRateRange on the WKWebView itself (public API, macOS 13.3+).
//      Sets minimum=80fps so ProMotion never drops to 60fps during idle periods.
//
// The post-creation plugin approach (tauri-plugin-macos-fps) uses the private
// `_features` array API. That selector was removed in macOS 26 and calling it
// throws "unrecognized selector" which aborts the process — avoid it.

// CAFrameRateRange mirrors CoreAnimation's public struct (3 x f32, macOS 12+).
// Used to set WKWebView.preferredFrameRateRange (macOS 13.3+).
#[cfg(target_os = "macos")]
#[repr(C)]
#[derive(Clone, Copy)]
struct CAFrameRateRange {
    minimum: f32,
    maximum: f32,
    preferred: f32,
}

// Static field array used by the Encode impl below.
// const { &[...] } works but a static makes the 'static lifetime explicit.
#[cfg(target_os = "macos")]
static CA_FRAME_RATE_RANGE_FIELDS: [objc2::encode::Encoding; 3] = [
    objc2::encode::Encoding::Float,
    objc2::encode::Encoding::Float,
    objc2::encode::Encoding::Float,
];

#[cfg(target_os = "macos")]
unsafe impl objc2::encode::Encode for CAFrameRateRange {
    const ENCODING: objc2::encode::Encoding =
        objc2::encode::Encoding::Struct("CAFrameRateRange", &CA_FRAME_RATE_RANGE_FIELDS);
}

#[cfg(target_os = "macos")]
unsafe impl objc2::encode::RefEncode for CAFrameRateRange {
    const ENCODING_REF: objc2::encode::Encoding =
        objc2::encode::Encoding::Pointer(&objc2::encode::Encoding::Struct(
            "CAFrameRateRange",
            &CA_FRAME_RATE_RANGE_FIELDS,
        ));
}

#[cfg(target_os = "macos")]
unsafe fn disable_webkit_60fps_cap_early() {
    use objc2::runtime::{AnyClass, AnyObject, Bool};
    use objc2::msg_send;

    let Some(defaults_cls) = AnyClass::get(c"NSUserDefaults") else { return };
    let defaults: *mut AnyObject = unsafe { msg_send![defaults_cls, standardUserDefaults] };
    if defaults.is_null() { return }

    let Some(str_cls) = AnyClass::get(c"NSString") else { return };

    for key in [
        b"WebKitPreferPageRenderingUpdatesNear60FPSEnabled\0" as &[u8],
        b"PreferPageRenderingUpdatesNear60FPSEnabled\0",
    ] {
        let nskey: *mut AnyObject =
            unsafe { msg_send![str_cls, stringWithUTF8String: key.as_ptr()] };
        if nskey.is_null() { continue }
        let _: () = unsafe { msg_send![defaults, setBool: Bool::new(false), forKey: nskey] };
    }
    log::info!("ProMotion: NSUserDefaults 60fps keys cleared");
}

// Post-creation fix: applied via with_webview in setup().
// Every call is guarded by respondsToSelector: — the only safe pattern inside
// a with_webview closure, which runs synchronously from did_finish_launching
// (extern "C" context where ObjC exceptions can't unwind → SIGABRT).
#[cfg(target_os = "macos")]
unsafe fn disable_webkit_60fps_cap_post(wkwebview_ptr: *mut objc2::runtime::AnyObject) {
    use objc2::runtime::{AnyObject, Bool};
    use objc2::msg_send;

    // Step A: remove the 60fps cap from WKPreferences.
    let config: *mut AnyObject = msg_send![wkwebview_ptr, configuration];
    if !config.is_null() {
        let prefs: *mut AnyObject = msg_send![config, preferences];
        if !prefs.is_null() {
            let sel = objc2::sel!(setPreferPageRenderingUpdatesNear60FPSEnabled:);
            let responds: Bool = msg_send![prefs, respondsToSelector: sel];
            if responds.as_bool() {
                let _: () = msg_send![prefs, setPreferPageRenderingUpdatesNear60FPSEnabled: Bool::new(false)];
                log::info!("ProMotion: 60fps cap removed via WKPreferences setter");
            } else {
                log::warn!("ProMotion: setPreferPageRenderingUpdatesNear60FPSEnabled: absent — 60fps cap may persist");
            }
        }
    }

    // Step B: lock the ProMotion rate to exactly 120fps.
    // preferredFrameRateRange (public API, macOS 13.3+) with minimum=120 prevents
    // the OS from ever running below 120fps, even when idle. This only takes effect
    // while WKWebView's internal display link is active — the persistent rAF loop
    // injected via CROWBAR_BOOTSTRAP keeps the link alive.
    // Falls back to the deprecated preferredFramesPerSecond: NSInteger on macOS < 13.3.
    let sel_pfr = objc2::sel!(setPreferredFrameRateRange:);
    let has_pfr: Bool = msg_send![wkwebview_ptr, respondsToSelector: sel_pfr];
    if has_pfr.as_bool() {
        let range = CAFrameRateRange { minimum: 120.0, maximum: 120.0, preferred: 120.0 };
        let _: () = msg_send![wkwebview_ptr, setPreferredFrameRateRange: range];
        log::info!("ProMotion: preferredFrameRateRange locked to 120fps");
    } else {
        let sel_fps = objc2::sel!(setPreferredFramesPerSecond:);
        let has_fps: Bool = msg_send![wkwebview_ptr, respondsToSelector: sel_fps];
        if has_fps.as_bool() {
            let _: () = msg_send![wkwebview_ptr, setPreferredFramesPerSecond: 120i64];
            log::info!("ProMotion: preferredFramesPerSecond set to 120 (pre-13.3 fallback)");
        }
    }
}

// Injected into the webview at document-start on every page load (including full
// reloads), before any frontend JS runs. It sets the API base the frontend
// resolves against (`api.ts` / `ws/url.ts`). Doing this as an init script rather
// than a one-time `setup()` eval matters: a reload wipes `window.__CROWBAR__`,
// and without it the frontend falls back to the dev origin and dials a doomed
// `ws://localhost:5173`, which flips the connection store to "disconnected" and
// flashes the "backend unavailable — reconnecting" banner. Guarded by hostname
// so it never leaks the global into browser-pane webviews showing external sites
// (the app itself is served from localhost / tauri.localhost).
//
// ProMotion note: a rAF loop with no DOM work does NOT keep the display at 120 Hz
// because WebKit skips GPU commits when nothing changes. The real fix is a CSS
// transform animation in index.html (a composited CA layer that continuously
// submits GPU frames, signalling ProMotion to hold 120 Hz).
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

pub fn run() {
    // Step 1: NSUserDefaults before WKWebView creation (macOS 13-15 path).
    #[cfg(target_os = "macos")]
    unsafe { disable_webkit_60fps_cap_early() }

    let mut builder = tauri::Builder::default()
        // tauri-plugin-macos-fps uses the `_features` private selector which was
        // removed in macOS 26. Keep it for macOS 13-15 compatibility but it is a
        // no-op (or crash-risk) on macOS 26 — our KVC fix in setup() covers that.
        .plugin(tauri_plugin_macos_fps::init())
        .plugin(tauri_plugin_log::Builder::new().build())
        .plugin(tauri_plugin_shell::init())
        // Inject the desktop API endpoint on every webview load (see CROWBAR_BOOTSTRAP).
        .plugin(
            tauri::plugin::Builder::<tauri::Wry, ()>::new("crowbar-bootstrap")
                .js_init_script(CROWBAR_BOOTSTRAP.to_string())
                .build(),
        )
        // Route webview `crowbar://localhost/v0/...` fetches through the unix
        // socket the daemon listens on (see sidecar + api_proxy).
        .register_asynchronous_uri_scheme_protocol("crowbar", api_proxy::handle_request)
        .register_asynchronous_uri_scheme_protocol("crowbar-browser", browser_proxy::handle_request);

    // Dev-only: exposes the webview to the Tauri MCP server (WebSocket :9223).
    // Gated to debug builds so it never ships in a release.
    #[cfg(debug_assertions)]
    {
        builder = builder.plugin(tauri_plugin_mcp_bridge::init());
    }

    builder
        .manage(sidecar::SidecarHandle::new())
        .manage(browser_pane::BrowserPaneManager::new())
        .manage(terminal::TerminalManager::new())
        .setup(|app| {
            let app_handle = app.handle().clone();

            // The daemon listens on a unix socket the webview reaches via the
            // `crowbar://` custom protocol (bridged by api_proxy). The endpoint
            // is injected into the webview by the crowbar-bootstrap init script
            // above, so nothing to do here for that.
            let socket = sidecar::socket_path();

            // Native macOS blur behind the transparent window. NSVisualEffectView
            // blur is fixed per material (no numeric radius); HudWindow is one of
            // the heaviest-blur materials. Requires `transparent: true` +
            // `macOSPrivateApi: true` (set in tauri.conf.json).
            #[cfg(target_os = "macos")]
            if let Some(window) = app.get_webview_window("main") {
                use window_vibrancy::{apply_vibrancy, NSVisualEffectMaterial, NSVisualEffectState};
                if let Err(e) = apply_vibrancy(
                    &window,
                    NSVisualEffectMaterial::HudWindow,
                    Some(NSVisualEffectState::FollowsWindowActiveState),
                    None,
                ) {
                    log::error!("failed to apply window vibrancy: {e}");
                }

                // Step 2: post-creation setter, guarded by respondsToSelector:.
                let _ = window.with_webview(|wv| {
                    use objc2::runtime::AnyObject;
                    let ptr = wv.inner() as *mut AnyObject;
                    unsafe { disable_webkit_60fps_cap_post(ptr) };
                });
            }

            // Spawn the Go daemon sidecar on the unix socket.
            tauri::async_runtime::spawn(async move {
                if let Err(e) = sidecar::spawn(&app_handle, socket).await {
                    log::error!("failed to start crowbar daemon: {e}");
                }
            });

            Ok(())
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
        .invoke_handler(tauri::generate_handler![
            browser_pane::browser_pane_sync,
            browser_pane::browser_pane_navigate,
            browser_pane::browser_pane_go_back,
            browser_pane::browser_pane_go_forward,
            browser_pane::browser_pane_reload,
            browser_pane::browser_pane_close,
            browser_pane::browser_pane_nav_event,
            terminal::terminal_open,
            terminal::terminal_send,
            terminal::terminal_resize,
            terminal::terminal_close,
        ])
        .run(tauri::generate_context!())
        .expect("error running Tauri app");
}
