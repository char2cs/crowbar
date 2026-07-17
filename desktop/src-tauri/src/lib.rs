mod api_proxy;
mod diagnostics;
mod fdlimit;
mod sidecar;
mod terminal;
mod ws_bridge;

#[cfg(test)]
mod test_support {
    use std::sync::OnceLock;
    use tokio::sync::{Mutex, MutexGuard};

    /// Serialises every test that opens a file descriptor.
    ///
    /// A descriptor leak can only be measured process-wide (`/dev/fd`), and cargo runs the
    /// suite in parallel — so a descriptor another test opens mid-measurement reads as a
    /// leak, and one it closes can hide a real one. Every test that opens *anything* — a
    /// socket, a log file, a zip — takes this, which is what makes the count attributable
    /// to the test doing the counting. A test that opens nothing does not need it.
    ///
    /// tokio's mutex rather than std's: it is held across awaits, and it does not poison,
    /// so one failing test cannot cascade into the rest.
    fn lock() -> &'static Mutex<()> {
        static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
        LOCK.get_or_init(|| Mutex::new(()))
    }

    pub async fn fd_tests() -> MutexGuard<'static, ()> {
        lock().lock().await
    }

    /// For the sync tests. `blocking_lock` panics inside a runtime, and these are not in
    /// one — that is precisely why they cannot use [`fd_tests`].
    pub fn fd_tests_blocking() -> MutexGuard<'static, ()> {
        lock().blocking_lock()
    }
}

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
//      Sets minimum=80fps for the frames the app actually produces. This raises
//      the rate of real rendering; it deliberately does NOT keep the display link
//      alive while content is static. Rendering is demand-driven: an idle window
//      must produce no frames, so ProMotion is free to drop. Do not "fix" an idle
//      refresh-rate reading by driving a permanent rAF/WebGL loop — that pins the
//      whole layer tree at ~120 commits/sec and costs ~65% of a core at idle.
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
const CA_FRAME_RATE_RANGE_ENCODING: objc2::encode::Encoding =
    objc2::encode::Encoding::Struct("CAFrameRateRange", &CA_FRAME_RATE_RANGE_FIELDS);

#[cfg(target_os = "macos")]
unsafe impl objc2::encode::Encode for CAFrameRateRange {
    const ENCODING: objc2::encode::Encoding = CA_FRAME_RATE_RANGE_ENCODING;
}

#[cfg(target_os = "macos")]
unsafe impl objc2::encode::RefEncode for CAFrameRateRange {
    const ENCODING_REF: objc2::encode::Encoding =
        objc2::encode::Encoding::Pointer(&CA_FRAME_RATE_RANGE_ENCODING);
}

#[cfg(target_os = "macos")]
unsafe fn disable_webkit_60fps_cap_early() {
    use objc2::msg_send;
    use objc2::runtime::{AnyClass, AnyObject, Bool};

    let Some(defaults_cls) = AnyClass::get(c"NSUserDefaults") else {
        return;
    };
    let defaults: *mut AnyObject = unsafe { msg_send![defaults_cls, standardUserDefaults] };
    if defaults.is_null() {
        return;
    }

    let Some(str_cls) = AnyClass::get(c"NSString") else {
        return;
    };

    for key in [
        b"WebKitPreferPageRenderingUpdatesNear60FPSEnabled\0" as &[u8],
        b"PreferPageRenderingUpdatesNear60FPSEnabled\0",
    ] {
        let nskey: *mut AnyObject =
            unsafe { msg_send![str_cls, stringWithUTF8String: key.as_ptr()] };
        if nskey.is_null() {
            continue;
        }
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
    use objc2::msg_send;
    use objc2::runtime::{AnyObject, Bool};

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
        let range = CAFrameRateRange {
            minimum: 120.0,
            maximum: 120.0,
            preferred: 120.0,
        };
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

/// macOS application menu, deliberately omitting three default accelerators so
/// the keystrokes reach the webview instead of being captured natively by AppKit
/// (menu key-equivalents are handled before the web content):
///   - Edit > Undo / Redo (Cmd+Z / Shift+Cmd+Z): the native Undo targets the
///     WKWebView's own undo, not Monaco's document undo stack. Omitting lets
///     Cmd+Z reach Monaco so editor undo works.
///   - Window > Close (Cmd+W): natively closes the window, quitting the app.
///     Omitting frees Cmd+W for the in-app "close active tab" keybinding
///     (web/src/features/panes/hooks/use-pane-keyboard.ts).
///
/// Everything else standard is kept: Cut/Copy/Paste/Select-All, Hide, Services,
/// Quit (Cmd+Q), Minimize.
#[cfg(target_os = "macos")]
fn build_app_menu(app: &tauri::AppHandle) -> tauri::Result<tauri::menu::Menu<tauri::Wry>> {
    use tauri::menu::{AboutMetadata, MenuBuilder, SubmenuBuilder};

    let app_menu = SubmenuBuilder::new(app, "Crowbar")
        .about(Some(AboutMetadata::default()))
        .separator()
        .services()
        .separator()
        .hide()
        .hide_others()
        .show_all()
        .separator()
        .quit()
        .build()?;

    // No undo()/redo(): freed for Monaco (see doc comment above).
    let edit_menu = SubmenuBuilder::new(app, "Edit")
        .cut()
        .copy()
        .paste()
        .select_all()
        .build()?;

    // No close_window(): Cmd+W is freed for the in-app close-active-tab binding.
    let window_menu = SubmenuBuilder::new(app, "Window").minimize().build()?;

    MenuBuilder::new(app)
        .items(&[&app_menu, &edit_menu, &window_menu])
        .build()
}

/// Reveal a file or directory in the OS file manager (Finder on macOS) with the
/// item selected. Calls tauri-plugin-opener's platform implementation as a plain
/// library function behind our own command: an app command is invokable without
/// any capability entry, whereas the plugin's FE-facing command would need a
/// path-glob scope broad enough to whitelist every workspace root (including
/// dot-segment paths like ~/.crowbar/projects/...), which the glob matcher
/// handles inconsistently. (Verified: this app has no `permissions/` dir, so no
/// ACL manifest exists under tauri_utils::acl::APP_ACL_KEY for it — `RuntimeAuthority`
/// only gates local, non-plugin commands when the app HAS one — see
/// tauri::webview's invoke dispatch. `cargo build`'s `gen/schemas/acl-manifests.json`
/// has no "app" key, confirming it today. If a `permissions/` dir is ever added for
/// some other command, this one must get an explicit allow alongside it.)
///
/// Must be `async`, unlike this rule's one other exception below
/// (`set_vibrancy_appearance`, which genuinely needs the main thread for its
/// AppKit view mutation): a plain, non-async `#[tauri::command]` runs INLINE on
/// the calling thread, which for a WKWebView IPC message is the app's main
/// thread (see `set_vibrancy_appearance`'s doc comment). `reveal_item_in_dir`'s
/// macOS path calls `NSWorkspace.activateFileViewerSelectingURLs`, a blocking
/// round trip to Finder over XPC. Run inline on the main thread, a slow round
/// trip stalls it — and because that same thread also pumps the WKWebView's
/// message loop, the whole script context freezes (not just this one invoke)
/// until it resolves. This is exactly the "invoke hangs the page" symptom Task
/// 30 reported; it is not an ACL denial (see above). Every other IPC command in
/// this file (terminal.rs, ws_bridge.rs, diagnostics.rs) is already `async` for
/// this reason — this one just wasn't (Task 28). `spawn_blocking` moves the
/// actual blocking call onto a dedicated blocking-pool thread so a slow Finder
/// round trip no longer holds up the UI.
#[tauri::command]
async fn reveal_in_finder(path: String) -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(move || {
        tauri_plugin_opener::reveal_item_in_dir(&path).map_err(|e| e.to_string())
    })
    .await
    .map_err(|e| format!("reveal_in_finder task panicked: {e}"))?
}

/// Pin the native vibrancy frost to a fixed appearance so it renders per-theme.
///
/// window-vibrancy 0.6.0 `apply_vibrancy` adds an NSVisualEffectView tagged
/// NS_VIEW_TAG_BLUR_VIEW = 91376254 (window-vibrancy-0.6.0/src/macos/internal.rs:13)
/// as a `Below` subview of the window contentView but never calls setAppearance:,
/// so the frost inherits effectiveAppearance from the OS (dark in BOTH themes —
/// the proven root cause of the light theme reading gray). Pinning the blur view
/// to NSAppearanceNameAqua (light) / NSAppearanceNameDarkAqua (dark) makes the
/// SAME HUDWindow material render light/dark by construction. Targets the blur
/// VIEW (falls back to the NSWindow), never NSApp, so the WKWebView's own
/// prefers-color-scheme is untouched.
#[tauri::command]
fn set_vibrancy_appearance(window: tauri::WebviewWindow, dark: bool) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        use objc2::msg_send;
        use objc2::runtime::{AnyClass, AnyObject, Bool};

        // window-vibrancy internal.rs:13 — the tag of the NSVisualEffectView.
        const NS_VIEW_TAG_BLUR_VIEW: isize = 91376254;

        unsafe {
            // AppKit is main-thread-only. Tauri runs sync commands on the main
            // (event-loop) thread; guard defensively so we never msg_send a UI
            // object off-main.
            let thread_cls = AnyClass::get(c"NSThread").ok_or("NSThread class missing")?;
            let is_main: Bool = msg_send![thread_cls, isMainThread];
            if !is_main.as_bool() {
                return Err("set_vibrancy_appearance must run on the main thread".into());
            }

            let ns_window = window
                .ns_window()
                .map_err(|e| format!("ns_window() failed: {e}"))?
                as *mut AnyObject;
            if ns_window.is_null() {
                return Err("ns_window is null".into());
            }
            let content_view: *mut AnyObject = msg_send![ns_window, contentView];
            if content_view.is_null() {
                return Err("contentView is null".into());
            }

            // The string VALUE of NSAppearanceNameAqua / NSAppearanceNameDarkAqua
            // equals the symbol name, so appearanceNamed: resolves it from a
            // plain NSString.
            let appearance_cls =
                AnyClass::get(c"NSAppearance").ok_or("NSAppearance class missing")?;
            let str_cls = AnyClass::get(c"NSString").ok_or("NSString class missing")?;
            let name_bytes: &[u8] = if dark {
                b"NSAppearanceNameDarkAqua\0"
            } else {
                b"NSAppearanceNameAqua\0"
            };
            let name: *mut AnyObject =
                msg_send![str_cls, stringWithUTF8String: name_bytes.as_ptr()];
            let appearance: *mut AnyObject = msg_send![appearance_cls, appearanceNamed: name];
            if appearance.is_null() {
                return Err("appearanceNamed: returned nil".into());
            }

            // Pin ONLY the blur view's appearance — NOT the NSWindow. Setting the
            // window appearance also flips the WKWebView's effectiveAppearance →
            // its `prefers-color-scheme`, which the JS `system` theme mode reads to
            // follow macOS; pinning the window froze that ("doesn't react to
            // light/dark"). Pinning just the blur view lightens the HudWindow frost
            // (Aqua) / darkens it (DarkAqua) while leaving the webview's
            // prefers-color-scheme tied to the OS, so system mode keeps following.
            //
            // viewWithTag: searches the receiver + descendants; contentView.tag==0.
            let blur_view: *mut AnyObject =
                msg_send![content_view, viewWithTag: NS_VIEW_TAG_BLUR_VIEW];
            if !blur_view.is_null() {
                let _: () = msg_send![blur_view, setAppearance: appearance];
                let _: () = msg_send![blur_view, setNeedsDisplay: Bool::new(true)];
            } else {
                // Fallback only if the tagged view is missing.
                let _: () = msg_send![ns_window, setAppearance: appearance];
            }

            let _: () = msg_send![content_view, setNeedsDisplay: Bool::new(true)];
        }
        Ok(())
    }
    #[cfg(not(target_os = "macos"))]
    {
        let _ = (window, dark);
        Ok(())
    }
}

pub fn run() {
    // Before anything opens a descriptor: this process proxies every frontend
    // request and every WebSocket to the daemon over its own unix sockets, and
    // macOS starts a GUI app at launchd's soft limit of 256. No logger exists this
    // early, so the outcome is reported from setup() below.
    let fd_limit = fdlimit::raise();

    // Step 1: NSUserDefaults before WKWebView creation (macOS 13-15 path).
    #[cfg(target_os = "macos")]
    unsafe {
        disable_webkit_60fps_cap_early()
    }

    let mut builder = tauri::Builder::default()
        // tauri-plugin-macos-fps uses the `_features` private selector which was
        // removed in macOS 26. Keep it for macOS 13-15 compatibility but it is a
        // no-op (or crash-risk) on macOS 26 — our KVC fix in setup() covers that.
        .plugin(tauri_plugin_macos_fps::init())
        .plugin(tauri_plugin_dialog::init())
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
        .register_asynchronous_uri_scheme_protocol("crowbar", api_proxy::handle_request);

    // Dev-only: exposes the webview to the Tauri MCP server (WebSocket :9223).
    // Gated to debug builds so it never ships in a release.
    #[cfg(debug_assertions)]
    {
        builder = builder.plugin(tauri_plugin_mcp_bridge::init());
    }

    // Custom macOS menu that frees Cmd+Z/Cmd+W from native capture so the webview
    // (Monaco undo, in-app close-active-tab) can handle them. See build_app_menu.
    #[cfg(target_os = "macos")]
    {
        builder = builder.menu(build_app_menu);
    }

    builder
        .manage(sidecar::SidecarHandle::new())
        .manage(terminal::TerminalManager::new())
        .manage(ws_bridge::WsBridgeManager::new())
        // A page load orphans every bridged connection the outgoing page owned: its JS
        // is gone and will never close ids it no longer remembers, and the new page
        // opens its own. Nothing else can notice — a `Channel` keeps working across a
        // reload, because a reloaded page is the same webview — so if we do not retire
        // them here, their reader tasks park on sockets nobody will ever read again and
        // hold a descriptor apiece for the life of the app. The daemon keeps each PTY
        // alive, so a reloaded page simply re-attaches the terminals it still wants.
        .on_page_load(|webview, payload| {
            if payload.event() != tauri::webview::PageLoadEvent::Started {
                return;
            }
            let app = webview.app_handle();
            app.state::<ws_bridge::WsBridgeManager>().close_all();
            app.state::<terminal::TerminalManager>().close_all();
        })
        .setup(move |app| {
            let app_handle = app.handle().clone();

            // Report the descriptor ceiling now that a logger exists. It is the
            // first number to reach for when the app cannot dial the daemon.
            match &fd_limit {
                fdlimit::Outcome::Failed(_) => log::warn!("{fd_limit}"),
                outcome => log::info!("{outcome}"),
            }

            // The daemon listens on a unix socket the webview reaches via the
            // `crowbar://` custom protocol (bridged by api_proxy). The endpoint
            // is injected into the webview by the crowbar-bootstrap init script
            // above, so nothing to do here for that.
            let socket = sidecar::socket_path();

            // Native macOS blur behind the transparent window. NSVisualEffectView
            // blur is fixed per material (no numeric radius). `HudWindow` maps to
            // NSVisualEffectMaterialHUDWindow — a heavy, smooth blur. Requires
            // `transparent: true` + `macOSPrivateApi: true` (set in tauri.conf.json).
            #[cfg(target_os = "macos")]
            if let Some(window) = app.get_webview_window("main") {
                use window_vibrancy::{
                    apply_vibrancy, NSVisualEffectMaterial, NSVisualEffectState,
                };
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

            // Supervise it: deep readiness probes, goroutine dump + restart on
            // a wedge (see sidecar::start_watchdog).
            sidecar::start_watchdog(app.handle().clone());

            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { .. } | tauri::WindowEvent::Destroyed = event
            {
                if let Some(state) = window.try_state::<sidecar::SidecarHandle>() {
                    // Tell the supervisor this exit is intentional so neither
                    // the output pump nor the watchdog respawns the daemon.
                    state
                        .shutting_down
                        .store(true, std::sync::atomic::Ordering::SeqCst);
                    if let Some(child) = state.child.lock().unwrap().take() {
                        // On Unix: send SIGTERM to allow the daemon's graceful
                        // shutdown path (Container.Close → Terminal.Shutdown →
                        // flush+persist) to run. Poll up to 3 s, then SIGKILL.
                        // On Windows: fall back to the existing SIGKILL-only path.
                        //
                        // Signals use the health-reported pid via libc, never
                        // CommandChild::pid()/kill() — those lock the
                        // shared_child mutex the shell plugin's wait thread
                        // holds while the child lives, deadlocking this path.
                        #[cfg(unix)]
                        {
                            match state.daemon_pid() {
                                Some(pid) => {
                                    let pid = pid as libc::pid_t;
                                    // SIGTERM — request orderly shutdown.
                                    unsafe { libc::kill(pid, libc::SIGTERM) };
                                    // Wait up to 3 s for the daemon to exit cleanly.
                                    let deadline = std::time::Instant::now()
                                        + std::time::Duration::from_secs(3);
                                    while std::time::Instant::now() < deadline {
                                        std::thread::sleep(std::time::Duration::from_millis(100));
                                        // kill(pid, 0) returns 0 while the process exists.
                                        if unsafe { libc::kill(pid, 0) } != 0 {
                                            break; // Process exited — no SIGKILL needed.
                                        }
                                    }
                                    // SIGKILL fallback — no-op (ESRCH) if already gone.
                                    unsafe { libc::kill(pid, libc::SIGKILL) };
                                    drop(child);
                                }
                                None => {
                                    // Daemon predating pid reporting: only the
                                    // (deadlock-prone) child handle remains.
                                    let _ = child.kill();
                                }
                            }
                        }
                        #[cfg(not(unix))]
                        {
                            let _ = child.kill();
                        }
                    }
                }
            }
        })
        .invoke_handler(tauri::generate_handler![
            terminal::terminal_open,
            terminal::terminal_send,
            terminal::terminal_resize,
            terminal::terminal_resync,
            terminal::terminal_set_theme,
            terminal::terminal_close,
            ws_bridge::ws_open,
            ws_bridge::ws_send,
            ws_bridge::ws_close,
            diagnostics::diagnostics_export,
            reveal_in_finder,
            set_vibrancy_appearance,
        ])
        .run(tauri::generate_context!())
        .expect("error running Tauri app");
}

#[cfg(test)]
mod reveal_in_finder_tests {
    use super::reveal_in_finder;

    // Regression for Task 30: `reveal_in_finder` must stay `async` so its
    // blocking, cross-process Finder call runs on a `spawn_blocking` thread
    // instead of inline on the caller — inline execution is what froze the
    // whole webview main thread (see the doc comment on `reveal_in_finder`).
    //
    // A nonexistent path fails at `std::fs::canonicalize` before the platform
    // `imp::reveal_items_in_dir` (the actual NSWorkspace/XPC call) ever runs,
    // so this stays safe and deterministic in CI: no Finder window, no
    // WindowServer dependency, no real IPC. It only exercises that the
    // `async fn` + `spawn_blocking` + `JoinHandle` plumbing still propagates
    // the underlying error correctly.
    #[tokio::test]
    async fn reveal_in_finder_is_async_and_propagates_errors() {
        let result = reveal_in_finder("/definitely/does/not/exist/crowbar-task-30".into()).await;
        assert!(
            result.is_err(),
            "canonicalize on a nonexistent path must fail"
        );
    }
}
