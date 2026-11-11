//! Native macOS window chrome: the vibrancy blur behind the transparent window, its
//! per-theme appearance, and the traffic-light inset. No-ops on other platforms.
//! The AppKit commands are sync on purpose: a sync command runs on the main thread.

/// Vibrancy blur plus the WebKit 60fps uncap, applied to EVERY window. State is
/// `Active`: an inactive NSVisualEffectView paints a flat fill, so the transparent
/// window would visibly lose its blur whenever it stops being key.
#[cfg(target_os = "macos")]
pub fn decorate_window(webview: &tauri::Webview) {
    let window = webview.window();
    use window_vibrancy::{apply_vibrancy, NSVisualEffectMaterial, NSVisualEffectState};

    if let Err(e) = apply_vibrancy(
        window,
        NSVisualEffectMaterial::HudWindow,
        Some(NSVisualEffectState::Active),
        None,
    ) {
        log::error!("failed to apply window vibrancy: {e}");
    }

    let _ = webview.with_webview(|wv| {
        use objc2::runtime::AnyObject;
        let ptr = wv.inner() as *mut AnyObject;
        unsafe { crate::frame_rate::uncap_webview(ptr) };
    });
}

#[cfg(not(target_os = "macos"))]
pub fn decorate_window(_webview: &tauri::Webview) {}

/// Pins the vibrancy view to Aqua / DarkAqua so the frost renders per-theme
/// (window-vibrancy never sets an appearance, so it follows the OS). Only the blur
/// view, never the window: that would freeze the webview's `prefers-color-scheme`.
#[tauri::command]
pub fn set_vibrancy_appearance(window: tauri::Window, dark: bool) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        use objc2::msg_send;
        use objc2::runtime::{AnyClass, AnyObject, Bool};

        // window-vibrancy vibrancy.rs:13 — the tag of the NSVisualEffectView.
        const NS_VIEW_TAG_BLUR_VIEW: isize = 91376254;

        unsafe {
            // AppKit is main-thread-only; never msg_send a UI object off-main.
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

            // The appearance names' string values equal their symbol names.
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

            // viewWithTag: searches the receiver + descendants; contentView.tag==0.
            let blur_view: *mut AnyObject =
                msg_send![content_view, viewWithTag: NS_VIEW_TAG_BLUR_VIEW];
            if !blur_view.is_null() {
                let _: () = msg_send![blur_view, setAppearance: appearance];
                let _: () = msg_send![blur_view, setNeedsDisplay: Bool::new(true)];
            } else {
                // No blur view means the translucent chrome sits over the raw desktop.
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

/// The title-bar container's new `(height, origin_y)` for a close button of the given
/// height inset by `y`; AppKit frames are bottom-left-origin. Mirrors wry's own
/// `inset_traffic_lights`, kept pure so it is testable without AppKit.
#[cfg_attr(not(test), allow(dead_code))] // reason: only the macOS command calls it outside tests
fn traffic_light_container_frame(
    close_button_height: f64,
    y: f64,
    window_frame_height: f64,
) -> (f64, f64) {
    let height = close_button_height + y;
    (height, window_frame_height - height)
}

/// Runtime counterpart to the config-only `trafficLightPosition`: tauri never exposes
/// its runtime setter, so this does what wry/tao do against the raw NSWindow.
/// `x`/`y` are logical points, the units of the config and `getBoundingClientRect()`.
#[tauri::command]
pub fn set_traffic_light_position(window: tauri::Window, x: f64, y: f64) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        use objc2::msg_send;
        use objc2::runtime::{AnyClass, AnyObject, Bool};
        use objc2_foundation::NSRect;

        const CLOSE_BUTTON: usize = 0;
        const MINIATURIZE_BUTTON: usize = 1;
        const ZOOM_BUTTON: usize = 2;

        unsafe {
            // See set_vibrancy_appearance's identical guard.
            let thread_cls = AnyClass::get(c"NSThread").ok_or("NSThread class missing")?;
            let is_main: Bool = msg_send![thread_cls, isMainThread];
            if !is_main.as_bool() {
                return Err("set_traffic_light_position must run on the main thread".into());
            }

            let ns_window = window
                .ns_window()
                .map_err(|e| format!("ns_window() failed: {e}"))?
                as *mut AnyObject;
            if ns_window.is_null() {
                return Err("ns_window is null".into());
            }

            let close: *mut AnyObject = msg_send![ns_window, standardWindowButton: CLOSE_BUTTON];
            let miniaturize: *mut AnyObject =
                msg_send![ns_window, standardWindowButton: MINIATURIZE_BUTTON];
            let zoom: *mut AnyObject = msg_send![ns_window, standardWindowButton: ZOOM_BUTTON];
            if close.is_null() || miniaturize.is_null() {
                return Err("traffic light buttons missing".into());
            }

            let close_superview: *mut AnyObject = msg_send![close, superview];
            if close_superview.is_null() {
                return Err("close button has no superview".into());
            }
            let title_bar_container: *mut AnyObject = msg_send![close_superview, superview];
            if title_bar_container.is_null() {
                return Err("title bar container missing".into());
            }

            let close_frame: NSRect = msg_send![close, frame];
            let window_frame: NSRect = msg_send![ns_window, frame];
            let (container_height, container_origin_y) =
                traffic_light_container_frame(close_frame.size.height, y, window_frame.size.height);

            let mut container_frame: NSRect = msg_send![title_bar_container, frame];
            container_frame.size.height = container_height;
            container_frame.origin.y = container_origin_y;
            let _: () = msg_send![title_bar_container, setFrame: container_frame];

            let miniaturize_frame: NSRect = msg_send![miniaturize, frame];
            let space_between = miniaturize_frame.origin.x - close_frame.origin.x;

            let mut buttons = vec![close, miniaturize];
            if !zoom.is_null() {
                buttons.push(zoom);
            }
            for (i, button) in buttons.into_iter().enumerate() {
                let mut frame: NSRect = msg_send![button, frame];
                frame.origin.x = x + (i as f64 * space_between);
                let _: () = msg_send![button, setFrameOrigin: frame.origin];
            }
        }
        Ok(())
    }
    #[cfg(not(target_os = "macos"))]
    {
        let _ = (window, x, y);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::traffic_light_container_frame;

    /// The static config's 44px row, flush with the top of an 800pt window.
    #[test]
    fn matches_static_config_baseline() {
        let (height, origin_y) = traffic_light_container_frame(14.0, 23.0, 800.0);
        assert_eq!(height, 37.0);
        // AppKit's frame is bottom-up: a container flush with the window top
        // sits at window_height - container_height.
        assert_eq!(origin_y, 800.0 - 37.0);
    }

    #[test]
    fn taller_inset_grows_the_container_and_lowers_its_origin() {
        let (short_height, short_origin_y) = traffic_light_container_frame(14.0, 23.0, 800.0);
        let (tall_height, tall_origin_y) = traffic_light_container_frame(14.0, 33.0, 800.0);
        assert!(tall_height > short_height);
        assert!(tall_origin_y < short_origin_y);
    }
}
