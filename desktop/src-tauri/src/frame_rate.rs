//! Lifts WKWebView's 60fps cap so ProMotion displays render the app at 120Hz.
//! Rendering stays demand-driven: never keep the display link alive with a permanent
//! rAF/WebGL loop — it pins the layer tree at ~120 commits/sec and burns a core at idle.

use objc2::encode::{Encode, Encoding, RefEncode};
use objc2::msg_send;
use objc2::runtime::{AnyClass, AnyObject, Bool};

/// CoreAnimation's public `CAFrameRateRange` (3 x f32, macOS 12+).
#[repr(C)]
#[derive(Clone, Copy)]
struct CAFrameRateRange {
    minimum: f32,
    maximum: f32,
    preferred: f32,
}

static CA_FRAME_RATE_RANGE_FIELDS: [Encoding; 3] =
    [Encoding::Float, Encoding::Float, Encoding::Float];

const CA_FRAME_RATE_RANGE_ENCODING: Encoding =
    Encoding::Struct("CAFrameRateRange", &CA_FRAME_RATE_RANGE_FIELDS);

unsafe impl Encode for CAFrameRateRange {
    const ENCODING: Encoding = CA_FRAME_RATE_RANGE_ENCODING;
}

unsafe impl RefEncode for CAFrameRateRange {
    const ENCODING_REF: Encoding = Encoding::Pointer(&CA_FRAME_RATE_RANGE_ENCODING);
}

/// Clears the 60fps preference in NSUserDefaults. Must run before any WKWebView is
/// created: macOS 13–15 WebKit reads it when building its display link.
pub unsafe fn uncap_before_webview() {
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

/// Uncaps a live WKWebView. Every call is guarded by `respondsToSelector:`: this runs
/// inside `with_webview` (an extern "C" context), where an ObjC exception cannot
/// unwind and aborts the process. Never use the private `_features` API (gone in macOS 26).
pub unsafe fn uncap_webview(wkwebview_ptr: *mut AnyObject) {
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

    // preferredFrameRateRange is public API from macOS 13.3; older systems only have
    // the deprecated preferredFramesPerSecond.
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
