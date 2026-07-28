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

/// Badged artwork worn only by builds running out of a developer's own tree.
///
/// `desktop/icons-src/crowbar-dev.png` carries ~10% transparent padding on all
/// four sides. That margin is LOAD-BEARING, not slack to be cropped: unlike the
/// bundled .icns/.icon paths, `-setApplicationIconImage:` hands AppKit a bitmap
/// it draws edge-to-edge with no inset of its own, so a tight crop renders
/// noticeably larger than every neighbouring Dock icon. Leave the file alone.
#[cfg(all(debug_assertions, target_os = "macos"))]
const DEV_ICON_PNG: &[u8] = include_bytes!("../../icons-src/crowbar-dev.png");

/// Give debug builds a visually distinct Dock / app-switcher icon.
///
/// Both dev entry points otherwise wear production Crowbar's face: `make
/// dev-desktop` (`tauri dev`) runs a bare binary with no bundle at all, and
/// `make dev-bundle` produces a .app carrying the same icon the release does.
/// With a real Crowbar usually also running, that makes the two instances
/// indistinguishable at a glance — the wrong window gets driven, and worse,
/// trusted (see the dev-isolation rules in the Makefile).
///
/// Swapping the icon at RUNTIME rather than through `bundle.icon` is what keeps
/// this off the release path entirely: `cfg(debug_assertions)` means the bytes
/// are not even linked into a release binary, and macOS release builds keep the
/// Icon Composer bundle (`icons/crowbar.icon` → Assets.car) untouched.
///
/// MUST be called from the `RunEvent::Ready` handler, never from `setup()`.
/// Tauri does this exact same call itself under `cfg(all(dev, target_os =
/// "macos"))` — see the `RuntimeRunEvent::Ready` arm in tauri's app.rs — feeding
/// it whichever `bundle.icon` entry ends in `.icns`. That fires after `setup()`,
/// so an icon set there is silently overwritten by `icons/icon.icns` moments
/// later; the app logs success and the Dock still shows the release artwork.
/// Ready reaches our callback only after tauri's own arm has run, which is what
/// makes it the first point where this sticks.
///
/// Best-effort by design: an app that fails to restyle its own icon has no
/// reason to refuse to boot, so every step degrades to leaving the icon as-is.
#[cfg(all(debug_assertions, target_os = "macos"))]
fn apply_dev_app_icon() {
    use objc2::msg_send;
    use objc2::runtime::{AnyClass, AnyObject};

    unsafe {
        let (Some(data_cls), Some(image_cls), Some(app_cls)) = (
            AnyClass::get(c"NSData"),
            AnyClass::get(c"NSImage"),
            AnyClass::get(c"NSApplication"),
        ) else {
            return;
        };

        // +dataWithBytes:length: copies, so the borrow of DEV_ICON_PNG ends here.
        let data: *mut AnyObject = msg_send![
            data_cls,
            dataWithBytes: DEV_ICON_PNG.as_ptr().cast::<std::ffi::c_void>(),
            length: DEV_ICON_PNG.len(),
        ];
        if data.is_null() {
            return;
        }

        // -initWithData: returns +1. We never release it: NSApp holds the icon
        // for the process lifetime anyway, so a matching release would only
        // hand us a dangling pointer. One leaked NSImage, once, in debug only.
        let image: *mut AnyObject = msg_send![image_cls, alloc];
        let image: *mut AnyObject = msg_send![image, initWithData: data];
        if image.is_null() {
            return;
        }

        let ns_app: *mut AnyObject = msg_send![app_cls, sharedApplication];
        if ns_app.is_null() {
            return;
        }

        let _: () = msg_send![ns_app, setApplicationIconImage: image];
    }
    log::info!("dev build: Dock icon set to badged dev artwork");
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

/// macOS application menu, deliberately omitting four default accelerators so
/// the keystrokes reach the webview instead of being captured natively by AppKit
/// (menu key-equivalents are handled before the web content):
///   - Edit > Undo / Redo (Cmd+Z / Shift+Cmd+Z): the native Undo targets the
///     WKWebView's own undo, not Monaco's document undo stack. Omitting lets
///     Cmd+Z reach Monaco so editor undo works.
///   - Edit > Select All (Cmd+A): the native Select All runs WebKit's `selectAll:`
///     on the focused element — Monaco's hidden input textarea, which (with
///     `editContext: false`, see monaco-adapters.ts) only ever holds a small buffer
///     around the cursor, NOT the whole document. So the native path selects that
///     partial buffer instead of the file. Omitting lets Cmd+A reach Monaco, whose
///     own select-all keybinding selects the full model
///     (web/src/features/editor/hooks/use-pane-editor-satellites.ts). Plain HTML
///     inputs keep Cmd+A via WebKit's built-in editing key bindings regardless of
///     this menu item, so nothing else regresses.
///   - Window > Close (Cmd+W): natively closes the window, quitting the app.
///     Omitting frees Cmd+W for the in-app "close active tab" keybinding
///     (web/src/features/panes/hooks/use-pane-keyboard.ts).
///
/// Everything else standard is kept: Cut/Copy/Paste, Hide, Services,
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

    // No undo()/redo()/select_all(): freed for Monaco (see doc comment above).
    // Cut/Copy/Paste stay — WebKit fires those as DOM events Monaco intercepts and
    // maps onto its model; Select All has no such interceptable event, so its
    // native accelerator would only ever select Monaco's partial input buffer.
    let edit_menu = SubmenuBuilder::new(app, "Edit")
        .cut()
        .copy()
        .paste()
        .build()?;

    // Still no close_window(): its accelerator is Cmd+W, which stays freed for the
    // in-app close-active-tab binding.
    //
    // Accelerators here must not collide with anything in the frontend keymap
    // registry (web/src/features/keymaps/registry.ts). A native menu accelerator is
    // consumed by AppKit BEFORE the keystroke reaches WKWebView, so a collision does
    // not "win a race" — it makes the in-app binding permanently unreachable, and
    // unrebindable too, since the settings capture never sees the key either. That
    // mechanism is why close_window() is omitted above.
    //
    // Both conventional New Window chords are already taken by in-app actions:
    // Cmd+N is `agent.newChat` and Cmd+Shift+N is `tab.newFile` (registry.ts). Window
    // management is far less frequent than either, so it does not get to evict them —
    // hence Cmd+Alt+N, which the registry leaves free.
    //
    // Close Window at Cmd+Shift+W (also free) is not optional decoration: with Cmd+W
    // rebound to close-tab, a keyboard-driven user otherwise has NO way to close a
    // window at all — only the traffic light. Fine for a one-window app, a real gap
    // for a multi-window one. Both are built by hand rather than via the predefined
    // items so they get these accelerators instead of the reserved defaults.
    let new_window = tauri::menu::MenuItemBuilder::new("New Window")
        .id(NEW_WINDOW_MENU_ID)
        .accelerator("CmdOrCtrl+Alt+N")
        .build(app)?;
    let close_window = tauri::menu::MenuItemBuilder::new("Close Window")
        .id(CLOSE_WINDOW_MENU_ID)
        .accelerator("CmdOrCtrl+Shift+W")
        .build(app)?;
    let window_menu = SubmenuBuilder::new(app, "Window")
        .item(&new_window)
        .item(&close_window)
        .separator()
        .minimize()
        .build()?;

    MenuBuilder::new(app)
        .items(&[&app_menu, &edit_menu, &window_menu])
        .build()
}

/// Menu id for the "New Window" item, shared between where it's built
/// (`build_app_menu`, above) and where it's matched (`on_menu_event` in `run`, ~285
/// lines away) so the two can't drift out of sync — a typo in either literal would
/// just make the menu item stop responding, with no compiler error and no runtime log.
#[cfg(target_os = "macos")]
const NEW_WINDOW_MENU_ID: &str = "new_window";

/// Menu id for "Close Window". Same drift argument as [`NEW_WINDOW_MENU_ID`].
#[cfg(target_os = "macos")]
const CLOSE_WINDOW_MENU_ID: &str = "close_window";

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
/// window-vibrancy `apply_vibrancy` adds an NSVisualEffectView tagged
/// NS_VIEW_TAG_BLUR_VIEW = 91376254 (window-vibrancy-0.8.0/src/macos/vibrancy.rs:13)
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

        // window-vibrancy vibrancy.rs:13 — the tag of the NSVisualEffectView.
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
                // Fallback only if the tagged view is missing — which means
                // apply_vibrancy never ran (or failed) for this window. Worth a
                // warning rather than a silent fallback: the window is
                // `transparent: true` and the chrome tokens are translucent on the
                // assumption this view is behind them, so its absence is not "no
                // blur", it is the desktop showing through the app. Silent fallback
                // is how that shipped once already, on windows built by open_window
                // — apply_vibrancy is main-thread-only and the command is async.
                log::warn!(
                    "vibrancy view missing on window '{}' — blur was never applied; \
                     chrome will render translucent over the desktop",
                    window.label()
                );
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

/// Native macOS chrome for a Crowbar window: the vibrancy blur behind the transparent
/// window plus the post-creation WebKit 60fps un-cap. Applied to EVERY window, not just
/// `main` — a second window is the same app and must look and animate identically.
///
/// NSVisualEffectView blur is fixed per material (no numeric radius). `HudWindow` maps to
/// NSVisualEffectMaterialHUDWindow — a heavy, smooth blur. Requires `transparent: true`
/// on the window (set in tauri.conf.json for `main`, mirrored in `open_window`'s builder
/// for every other window) and `macOSPrivateApi: true` app-wide (set once in
/// tauri.conf.json — it is a process entitlement, not a per-window builder option, so
/// every `WebviewWindowBuilder`-made window gets it automatically).
#[cfg(target_os = "macos")]
fn decorate_window(window: &tauri::WebviewWindow) {
    use window_vibrancy::{apply_vibrancy, NSVisualEffectMaterial, NSVisualEffectState};

    if let Err(e) = apply_vibrancy(
        window,
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

#[cfg(not(target_os = "macos"))]
fn decorate_window(_window: &tauri::WebviewWindow) {}

/// Builds the `initialization_script` that seeds `location.hash` to `route` on a new
/// window's first paint (see the guard rationale on `open_window`, which is the only
/// caller). Pulled out as a pure, synchronous function — no `AppHandle`, no webview —
/// so the guard's shape and its handling of adversarial `route` input are both
/// unit-testable without spinning up a real window.
///
/// `route` is embedded via `serde_json::to_string`, not a raw `format!` interpolation:
/// a workspace route is attacker-adjacent-enough data (round-tripped through routing
/// state, not a compile-time constant) that a hand-rolled JS string literal would be a
/// script-injection bug the moment a route contained a `"` or `\`. `serde_json`'s
/// escaping happens to produce a syntactically valid JS string literal too — its
/// escape rules for `"`, `\`, and control characters (including newlines, which are
/// illegal unescaped inside a single-line JS string literal) are exactly JSON's.
///
/// The guard fires only when `location.hash` is empty — this window's genuine first
/// load, before the hash router has ever run. It deliberately does NOT also match
/// `'#/'`: the root route is a real, user-reachable location (e.g. after navigating
/// back to the picker), not a stand-in for "nothing has happened yet". Matching it too
/// would re-seed `route` on every reload after the user deliberately returns to the
/// picker, silently overriding their navigation — the opposite of this guard's job.
fn seed_hash_script(route: &str) -> Result<String, String> {
    let encoded = serde_json::to_string(route).map_err(|e| format!("encode route: {e}"))?;
    Ok(format!(
        "if (!location.hash) {{ location.hash = {encoded}; }}"
    ))
}

/// Opens another Crowbar window at `route` (a router path such as
/// `/ide/<projectId>/<repoId>/<wsId>`, or `/` for the picker).
///
/// Every window is the same app: same sidebar, same everything. What differs is only
/// which workspace it is looking at, and that is pure routing — the frontend uses hash
/// history, so the target is delivered by seeding `location.hash` from an init script
/// (`seed_hash_script`) that runs before any app JS, on every load of this window
/// including reloads. Its guard fires only on a genuinely empty hash — this window's
/// first paint — so a later reload keeps wherever the user has since navigated,
/// including back to the root route; see `seed_hash_script`'s doc comment for why the
/// guard is narrow rather than also matching `'#/'`.
///
/// The window is built from tauri.conf.json's `app.windows[0]` with only the label
/// changed, which is the pattern tauri's own `from_config` docs prescribe for multiple
/// windows. That config describes only the FIRST window and a builder-made window
/// inherits none of it, so the options were previously re-stated by hand — and the
/// hand-written mirror was NOT equivalent, in a way that is invisible from the Rust
/// side. `WebviewWindowBuilder::traffic_light_position` writes to the WEBVIEW
/// attributes only (tauri-2.11.2 `webview/webview_window.rs:736`), whereas
/// `from_config` populates the window attributes too (`WindowBuilder::from_config` →
/// `tauri-runtime-wry` lib.rs:873). Those feed two independent macOS implementations
/// of the same inset: wry's `WryWebViewParent` re-applies it from ITS `drawRect:`, tao
/// re-applies it from the tao content view's. The tao view is the one that actually
/// redraws under a webview-filled window, so a window with only the webview-side inset
/// gets it applied once at webview creation — against a window frame height that is not
/// yet final — and then never corrected. That is the whole bug: secondary windows'
/// traffic lights sat at a different offset from the main window's. Deriving from the
/// config keeps the two paths identical by construction, and any future change to the
/// window config now reaches secondary windows for free.
///
/// `async` with no `.await` in the body is deliberate, not dead ceremony: Tauri's own
/// docs for `WebviewWindowBuilder::new` warn that on Windows, building a window from a
/// synchronous command or event handler deadlocks (tauri-2.11.2's
/// `src/webview/webview_window.rs:58`, "you should use `async` commands and separate
/// threads when creating windows"), and per-command handling already runs sync
/// commands inline on the caller's thread — the main thread, for IPC (see
/// `reveal_in_finder`'s doc comment for the general rule). Marking this `async` keeps
/// window creation off the main thread on every platform, which is also why the
/// `on_menu_event` caller `tauri::async_runtime::spawn`s it rather than blocking the
/// menu event loop.
#[tauri::command]
async fn open_window(app: tauri::AppHandle, route: String) -> Result<(), String> {
    use std::sync::atomic::{AtomicU64, Ordering};
    static NEXT_WINDOW: AtomicU64 = AtomicU64::new(2);

    let label = format!("w{}", NEXT_WINDOW.fetch_add(1, Ordering::Relaxed));
    let seed_hash = seed_hash_script(&route)?;

    // Only the label differs. `create: false` is not set on this config entry, so the
    // window it describes is also the one tauri made at startup and `label` MUST be
    // overridden — two live windows cannot share one.
    let mut config = tauri::Manager::config(&app)
        .app
        .windows
        .first()
        .cloned()
        .ok_or_else(|| {
            log::error!("open_window: tauri.conf.json declares no windows");
            "no window config to clone".to_string()
        })?;
    config.label = label;

    let window = tauri::WebviewWindowBuilder::from_config(&app, &config)
        .map_err(|e| {
            log::error!("open_window: config is not a valid window: {e}");
            format!("open window: {e}")
        })?
        .initialization_script(&seed_hash)
        .build()
        .map_err(|e| {
            log::error!("open_window: build failed: {e}");
            format!("open window: {e}")
        })?;

    // Must hop to the main thread. `apply_vibrancy` opens with a hard
    // `MainThreadMarker::new().ok_or(Error::NotMainThread)` gate
    // (window-vibrancy 0.6.0, macos/internal.rs:22), and this command is `async`,
    // so tauri dispatches it onto the multi-threaded tokio runtime — the menu path
    // spawns too. Called directly from here it fails EVERY time, and
    // `decorate_window` only logs the error.
    //
    // That is not a cosmetic "no blur": the window is `transparent: true` and the
    // chrome tokens are deliberately translucent (--chrome-bg is a color-mix with
    // `transparent`, see web/src/styles/theme.css) precisely because they sit on
    // the NSVisualEffectView. With no vibrancy view behind them the second window
    // shows the raw desktop through its own chrome. A webview screenshot cannot
    // see this — it captures no native chrome — which is exactly how it survived
    // live verification.
    //
    // The sibling `with_webview` call inside decorate_window is safe off-main on
    // its own (it proxies through send_user_message), but running the whole thing
    // on main is simpler than splitting them.
    let decorated = window.clone();
    window
        .run_on_main_thread(move || decorate_window(&decorated))
        .map_err(|e| {
            log::error!("open_window: could not decorate on the main thread: {e}");
            format!("decorate window: {e}")
        })?;
    Ok(())
}

/// Does any window OTHER than `closing` still exist?
///
/// This is the whole last-window decision, extracted so it can be tested — the
/// handler it serves is an inline closure inside [`run`], reachable only from a live
/// event loop, and it is the highest-consequence branch in the window lifecycle: too
/// eager and the daemon dies under a surviving window, too shy and it is orphaned.
///
/// It takes labels rather than an `AppHandle` for a reason beyond testability. The
/// obvious implementation, `webview_windows().len() > 1`, is WRONG, because whether
/// the closing window is still in that map depends on which event we are in.
/// `webview_windows()` reads tauri's AppManager maps and `AppManager::on_window_close`
/// prunes them: for `CloseRequested` that never runs (window still counted), for
/// `Destroyed` the RunEvent callback runs it synchronously BEFORE per-window listeners
/// (window already gone). The handler matches both, so any length threshold is wrong
/// for one of them — `> 1` kills the daemon on a non-last `Destroyed`. Asking "is
/// there another label" is immune to the asymmetry, and the tests below pin both
/// event shapes.
fn another_window_survives<'a>(labels: impl Iterator<Item = &'a str>, closing: &str) -> bool {
    labels.into_iter().any(|label| label != closing)
}

/// Stop the Go daemon sidecar: SIGTERM, wait up to 3 s for its graceful shutdown
/// (Container.Close → Terminal.Shutdown → flush+persist), then SIGKILL.
///
/// Called from BOTH ways the app can end, because they are genuinely different code
/// paths in tao and only one of them was covered before:
///
///   1. the last window closing (`on_window_event`), and
///   2. `RunEvent::Exit` — which is what Cmd+Q produces.
///
/// Cmd+Q is `NSApplication.terminate:`, and for a non-document app AppKit does NOT
/// close windows individually on the way out: tao registers only
/// `applicationWillTerminate:`, which goes straight to `AppState::exit()` →
/// `Event::LoopDestroyed` → `RunEvent::Exit`. No `Destroyed` per window, so path (1)
/// never fired and the daemon was left running — holding its socket, so the NEXT
/// launch's daemon refuses to bind and dies with code 1. `tauri-plugin-shell`'s own
/// exit sweep does not cover this: it only reaps children registered by its JS
/// `spawn` command, and ours comes from `sidecar::spawn` on the Rust side.
///
/// Idempotent. `child.lock().take()` yields `None` on the second call, so the last
/// window closing followed by `Exit` signals once, not twice.
fn shutdown_sidecar(app: &tauri::AppHandle, reason: &str) {
    let Some(state) = app.try_state::<sidecar::SidecarHandle>() else {
        return;
    };

    // Tell the supervisor this exit is intentional so neither the output pump nor
    // the watchdog respawns the daemon.
    state
        .shutting_down
        .store(true, std::sync::atomic::Ordering::SeqCst);

    let Some(child) = state.child.lock().unwrap().take() else {
        // Already stopped by the other ending. Expected — a last-window close is
        // followed by Exit — so this is not a warning.
        log::debug!("daemon shutdown ({reason}): already stopped");
        return;
    };

    // Worth an INFO line in production: "did the daemon get stopped, and by which
    // ending" is the first question when a launch reports a socket already in use.
    log::info!(
        "stopping crowbar daemon ({reason}, pid {:?})",
        state.daemon_pid()
    );

    // Signals use the health-reported pid via libc, never CommandChild::pid()/kill()
    // — those lock the shared_child mutex the shell plugin's wait thread holds while
    // the child lives, deadlocking this path.
    #[cfg(unix)]
    {
        match state.daemon_pid() {
            Some(pid) => {
                let pid = pid as libc::pid_t;
                unsafe { libc::kill(pid, libc::SIGTERM) };
                let deadline = std::time::Instant::now() + std::time::Duration::from_secs(3);
                // Poll BEFORE the first sleep: a daemon that exits promptly is the
                // normal case, and checking first keeps a clean shutdown off the
                // 100 ms floor. This blocks the event loop — during a window close
                // the window is still on screen, and under RunEvent::Exit it blocks
                // inside applicationWillTerminate: — so the budget is deliberately
                // 3 s, well inside AppKit's quit allowance and far better than the
                // orphaned daemon the wait exists to prevent.
                loop {
                    // kill(pid, 0) returns 0 while the process exists.
                    if unsafe { libc::kill(pid, 0) } != 0 {
                        break; // Exited cleanly — no SIGKILL needed.
                    }
                    if std::time::Instant::now() >= deadline {
                        break;
                    }
                    std::thread::sleep(std::time::Duration::from_millis(100));
                }
                // SIGKILL fallback — no-op (ESRCH) if already gone.
                unsafe { libc::kill(pid, libc::SIGKILL) };
                drop(child);
            }
            None => {
                // No pid recorded for this child. The live case is a daemon that is
                // still booting — `spawn` stores the child immediately but only
                // records the pid once `wait_for_health` returns — plus a daemon
                // predating pid reporting. Either way the child handle is all we
                // have, so this path is an ungraceful kill rather than SIGTERM's
                // flush-and-persist. `kill()` does not block: shared_child's wait
                // thread does not hold the lock while waiting.
                let _ = child.kill();
            }
        }
    }
    #[cfg(not(unix))]
    {
        let _ = child.kill();
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
        // Default the log plugin to Info (it defaults to Trace, which drowns the
        // output in the WebSocket stack's per-frame internals). Crowbar's own code
        // only logs at info/warn/error, so nothing of ours is lost. Explicitly cap
        // the chattiest dependency crates so a later global bump to debug/trace for
        // our own code doesn't re-open the firehose.
        .plugin(
            tauri_plugin_log::Builder::new()
                .level(log::LevelFilter::Info)
                .level_for("tokio_tungstenite", log::LevelFilter::Warn)
                .level_for("tungstenite", log::LevelFilter::Warn)
                .level_for("hyper", log::LevelFilter::Warn)
                .level_for("hyper_util", log::LevelFilter::Warn)
                .level_for("reqwest", log::LevelFilter::Warn)
                .level_for("mio", log::LevelFilter::Warn)
                .build(),
        )
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
        builder = builder.on_menu_event(|app, event| {
            if event.id() == NEW_WINDOW_MENU_ID {
                let app = app.clone();
                // The command is async and menu events are sync: spawn so the menu
                // handler returns immediately rather than blocking the event loop.
                tauri::async_runtime::spawn(async move {
                    if let Err(e) = open_window(app, "/".to_string()).await {
                        log::error!("New Window menu item failed: {e}");
                    }
                });
            } else if event.id() == CLOSE_WINDOW_MENU_ID {
                // Closes the FOCUSED window, which is what a Window-menu command
                // means. `close()` requests a close, so it runs the same
                // CloseRequested path as the traffic light — transports retired,
                // daemon stopped only if this was the last window.
                match app.get_focused_window() {
                    Some(window) => {
                        if let Err(e) = window.close() {
                            log::error!("Close Window menu item failed: {e}");
                        }
                    }
                    // No key window — e.g. every window minimised while the app is
                    // still frontmost. AppKit disables its own Close in that state;
                    // this item stays enabled, so say why nothing happened.
                    None => log::debug!("Close Window: no focused window to close"),
                }
            }
        });
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
            app.state::<ws_bridge::WsBridgeManager>()
                .close_for_window(webview.label());
            app.state::<terminal::TerminalManager>()
                .close_for_window(webview.label());
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

            // Native macOS blur + WebKit 60fps un-cap. See decorate_window — applied
            // here for `main`, and again for every window `open_window` creates.
            #[cfg(target_os = "macos")]
            if let Some(window) = app.get_webview_window("main") {
                decorate_window(&window);
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
                let label = window.label();

                // Retire this window's transports FIRST, whether or not it is the last
                // window. A closing window orphans its connections exactly as a
                // reloading one does — its JS is gone and will never call `ws_close` or
                // `terminal_close` — and `on_page_load` cannot cover it, because a
                // window that closes never loads a page again.
                //
                // This did not matter while any window's close took the whole app down
                // with it. Now that a non-last close returns early below, a stranded
                // connection would park its reader task on a socket for the life of the
                // process — and this app runs into macOS's 256-descriptor ceiling for
                // real (see fdlimit). Retiring is idempotent, so the second pass for the
                // same window (CloseRequested then Destroyed) is a no-op.
                let app = window.app_handle();
                app.state::<ws_bridge::WsBridgeManager>()
                    .close_for_window(label);
                app.state::<terminal::TerminalManager>()
                    .close_for_window(label);

                // The sidecar is app-wide, so only the LAST window closing may take it
                // down — see another_window_survives for why this is a label check and
                // not `webview_windows().len() > 1`.
                let windows = app.webview_windows();
                if another_window_survives(windows.keys().map(String::as_str), label) {
                    return;
                }

                shutdown_sidecar(app, "last window closed");
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
            open_window,
        ])
        .build(tauri::generate_context!())
        .expect("error building Tauri app")
        // Split out of `.run(context)` so we get a run-event callback: the dev
        // icon can only be applied once Ready has been through tauri's own
        // icon-setting arm. See apply_dev_app_icon.
        .run(|app_handle, event| {
            #[cfg(all(debug_assertions, target_os = "macos"))]
            if matches!(event, tauri::RunEvent::Ready) {
                apply_dev_app_icon();
            }

            // Cmd+Q does NOT close windows individually (see shutdown_sidecar), so the
            // on_window_event path never fires for it and the daemon would be orphaned
            // — holding its socket, which makes the next launch's daemon fail to bind.
            // tauri invokes this callback BEFORE cleanup_before_exit, so the handle is
            // still live here. Idempotent with the last-window path.
            if matches!(event, tauri::RunEvent::Exit) {
                shutdown_sidecar(app_handle, "app exit");
            }
        });
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

#[cfg(test)]
mod last_window_tests {
    use super::another_window_survives;

    /// The two shapes the SAME close produces, because `on_window_event` matches both
    /// `CloseRequested` and `Destroyed` and tauri prunes its window map between them.
    /// A `webview_windows().len() > 1` guard passes the first of these and FAILS the
    /// second — it sees `{main}`, concludes "no other window", and kills the daemon
    /// under `main`. That regression is the reason this function exists, so both
    /// shapes are pinned here rather than only the intuitive one.
    #[test]
    fn a_non_last_window_closing_leaves_the_daemon_alone_in_both_event_shapes() {
        // CloseRequested(w2): tauri has not pruned yet, so w2 is still listed.
        assert!(
            another_window_survives(["main", "w2"].into_iter(), "w2"),
            "CloseRequested: main survives w2, so the daemon must stay up"
        );

        // Destroyed(w2): tauri pruned w2 before dispatching to us. This is the case
        // a length threshold gets wrong.
        assert!(
            another_window_survives(["main"].into_iter(), "w2"),
            "Destroyed: main survives w2, so the daemon must stay up"
        );
    }

    /// The mirror: the daemon MUST come down when the closing window is the last one,
    /// or quitting leaves an orphan holding the socket and the next launch's daemon
    /// cannot bind.
    #[test]
    fn the_last_window_closing_takes_the_daemon_down_in_both_event_shapes() {
        // CloseRequested(main), sole window: still listed, and it is the one closing.
        assert!(
            !another_window_survives(["main"].into_iter(), "main"),
            "CloseRequested: nothing survives main, so the daemon must stop"
        );

        // Destroyed(main), sole window: already pruned, so the map is empty.
        assert!(
            !another_window_survives(std::iter::empty(), "main"),
            "Destroyed: nothing survives main, so the daemon must stop"
        );
    }

    /// Three windows, middle one closing — the guard must not be accidentally
    /// tuned to "exactly two".
    #[test]
    fn several_surviving_windows_all_keep_the_daemon_up() {
        assert!(another_window_survives(
            ["main", "w2", "w3"].into_iter(),
            "w2"
        ));
        assert!(another_window_survives(["main", "w3"].into_iter(), "w2"));
    }
}

#[cfg(test)]
mod seed_hash_script_tests {
    use super::seed_hash_script;

    /// Pins the reload guard's exact shape. The guard must fire only when
    /// `location.hash` is genuinely empty (this window's first paint) and must
    /// NOT also match `'#/'` — a review found that widening it to `'#/'` would
    /// re-seed `route` on every reload after the user deliberately navigates
    /// back to the root route, silently undoing their navigation. See the doc
    /// comment on `seed_hash_script` for the full rationale. Any future edit to
    /// the guard's condition has to go through this assertion.
    #[test]
    fn guard_fires_only_on_genuinely_empty_hash() {
        let script = seed_hash_script("/ide/p/r/w").expect("encode should succeed");
        assert_eq!(
            script,
            "if (!location.hash) { location.hash = \"/ide/p/r/w\"; }"
        );
        assert!(
            !script.contains("'#/'") && !script.contains("\"#/\""),
            "guard must not special-case the root route: {script}"
        );
    }

    /// "/" is the route the shipped caller actually passes — the New Window menu
    /// item opens the picker, not a workspace. It is also the route the removed
    /// `'#/'` guard arm used to reason about, so it is the one most likely to be
    /// "special-cased" again by a future edit. Pin it separately from the
    /// workspace-route case above.
    #[test]
    fn the_root_route_is_seeded_like_any_other() {
        let script = seed_hash_script("/").expect("encode should succeed");
        assert_eq!(script, "if (!location.hash) { location.hash = \"/\"; }");
    }

    /// A route reaching `open_window` is routing state, not a compile-time
    /// constant, so it has to be treated as untrusted input to the JS string
    /// literal this function builds. This proves a quote, a backslash, and a
    /// raw newline all round-trip through `serde_json`'s escaping into a
    /// syntactically valid JS string literal instead of breaking out of it.
    #[test]
    fn special_characters_round_trip_into_a_valid_js_string_literal() {
        let route = "/ide/\"quote\"/back\\slash/new\nline";
        let script = seed_hash_script(route).expect("encode should succeed");

        let prefix = "if (!location.hash) { location.hash = ";
        let suffix = "; }";
        assert!(
            script.starts_with(prefix) && script.ends_with(suffix),
            "unexpected script shape: {script}"
        );
        let literal = &script[prefix.len()..script.len() - suffix.len()];

        // An unescaped newline inside a single-line JS string literal is a
        // syntax error, not just a semantic bug — assert it was escaped away.
        assert!(
            !literal.contains('\n'),
            "raw newline leaked into the script: {script}"
        );

        // JSON and JS single-line string-literal escaping coincide for `"`,
        // `\`, and control characters, so parsing the embedded literal back as
        // JSON recovers exactly what a JS engine would parse it into.
        let decoded: String =
            serde_json::from_str(literal).expect("literal must be valid JSON/JS string syntax");
        assert_eq!(decoded, route);
    }
}
