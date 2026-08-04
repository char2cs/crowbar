//! The Crowbar-React window's native macOS chrome, ported from
//! `desktop/src-tauri/src/lib.rs` (item S0.5, spec §5.4): the `HudWindow`
//! vibrancy blur behind a transparent window, and the appearance pin that
//! keeps its frost per-theme instead of always following the OS.
//!
//! `window-vibrancy` supplies the first half — `apply_vibrancy` below calls
//! straight through to it and is itself a **safe** function, because
//! `window_vibrancy::apply_vibrancy` is safe at its own call boundary (the
//! unsafe `AppKit` work is inside it, gated on its own internal
//! `MainThreadMarker::new()` check). It does not supply the second half:
//! the `NSVisualEffectView` it inserts never has `setAppearance:` sent to it,
//! so left alone the frost inherits `effectiveAppearance` from the OS — dark
//! in both light and dark mode. [`pin_appearance`] is this item's own
//! hand-written unsafe, because nothing upstream does this.
//!
//! Both functions take `impl HasWindowHandle` rather than a `gpui::Window`
//! directly, keeping this crate a leaf with no edge onto `gpui` — the spike
//! recorded in spec §5.4 established that `gpui::Window` (and `&gpui::Window`,
//! via `raw_window_handle`'s blanket `impl<H: HasWindowHandle> HasWindowHandle
//! for &H`) already satisfies the bound, so no adapter type is needed at the
//! call site in `crowbar-app`.

use std::fmt;

use objc2::MainThreadMarker;
use objc2_app_kit::{
    NSAppearance, NSAppearanceCustomization, NSAppearanceNameAqua, NSAppearanceNameDarkAqua, NSView,
};
use raw_window_handle::{HandleError, HasWindowHandle, RawWindowHandle};

/// `window-vibrancy` `macos/vibrancy.rs:13`'s own comment: "`NSView::tag` for
/// `NSVisualEffectViewTagged`, just a random number." Not re-exported by the
/// crate (only the `apply_vibrancy`/`clear_vibrancy` functions and the
/// `NSVisualEffectViewTagged` type are public), so the literal is pinned
/// here too — the same way `desktop/src-tauri/src/lib.rs`'s
/// `set_vibrancy_appearance` already pins it, citing the same source line.
const NS_VIEW_TAG_BLUR_VIEW: isize = 91_376_254;

/// Applies the Crowbar-React window's blur material behind `window`: a heavy,
/// smooth `HudWindow` material (`NSVisualEffectMaterial::HudWindow`) whose
/// vibrancy state follows the window's own active/inactive state
/// (`NSVisualEffectState::FollowsWindowActiveState`), matching
/// `desktop/src-tauri/src/lib.rs`'s `decorate_window` exactly. `window` must
/// already be transparent (`WindowBackgroundAppearance::Transparent`, set at
/// window-creation time) — this only inserts the effect view behind it, it
/// does not make the window transparent itself.
///
/// No `unsafe` here: `window_vibrancy::apply_vibrancy` is a safe function.
///
/// # Errors
///
/// `window_vibrancy::Error` — `window` has no `AppKit` raw handle (wrong
/// platform), or `apply_vibrancy` was not called from the main thread, or the
/// running macOS is older than 10.10.
pub fn apply_vibrancy(window: impl HasWindowHandle) -> Result<(), window_vibrancy::Error> {
    window_vibrancy::apply_vibrancy(
        window,
        window_vibrancy::NSVisualEffectMaterial::HudWindow,
        Some(window_vibrancy::NSVisualEffectState::FollowsWindowActiveState),
        None,
    )
}

/// Why [`pin_appearance`] could not pin the frost.
#[derive(Debug)]
pub enum PinAppearanceError {
    /// `window` could not hand back a raw window handle at all.
    NoHandle(HandleError),
    /// `window`'s raw handle exists but is not an `AppKit` handle —
    /// meaningless off macOS, and a sign of a platform mismatch on it.
    NotAppKit,
    /// Called from a thread that is not the main thread. `AppKit` message
    /// sends are main-thread-only; every send this function makes is refused
    /// before it happens rather than attempted and hoped safe.
    NotMainThread,
}

impl fmt::Display for PinAppearanceError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::NoHandle(err) => write!(f, "no window handle: {err}"),
            Self::NotAppKit => write!(f, "the window handle is not an AppKit handle"),
            Self::NotMainThread => {
                write!(f, "pin_appearance must run on the main thread")
            }
        }
    }
}

impl std::error::Error for PinAppearanceError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::NoHandle(err) => Some(err),
            Self::NotAppKit | Self::NotMainThread => None,
        }
    }
}

/// Pins the native vibrancy frost [`apply_vibrancy`] inserted behind `window`
/// to a fixed appearance (`NSAppearanceNameAqua` when `dark` is `false`,
/// `NSAppearanceNameDarkAqua` when it is `true`), so the same `HudWindow`
/// material renders light or dark by construction instead of always
/// following the OS appearance.
///
/// Ported from `desktop/src-tauri/src/lib.rs`'s `set_vibrancy_appearance`.
/// Targets the tagged blur view itself — found by walking `window`'s own
/// `NSView` for a tagged descendant, `viewWithTag:`'s documented behaviour —
/// falling back to the window if the tagged view is not present (meaning
/// [`apply_vibrancy`] never ran or failed for this window, which the caller
/// should already have logged). Crowbar has no `prefers-color-scheme`
/// concept to protect here the way the Tauri reference's `WKWebView` did —
/// GPUI paints its own theme from `crowbar_ui::Theme`, nothing reads the
/// window's `NSAppearance` — so unlike the reference this function is free
/// to pin either the view or the window; it still prefers the view, kept
/// identical to the ported mechanism rather than diverging for a difference
/// that carries no behaviour here.
///
/// # Errors
///
/// [`PinAppearanceError::NoHandle`] if `window` cannot produce a raw handle
/// at all, [`PinAppearanceError::NotAppKit`] if it produces one but not an
/// `AppKit` one, [`PinAppearanceError::NotMainThread`] if this is not called
/// from the main thread — checked and refused before any `AppKit` message
/// send is attempted, see [Safety](#safety) below.
///
/// # Safety
///
/// Two `unsafe` operations, both inside the single block below, both
/// discharged by the same three facts:
///
/// 1. **The pointer is live and correctly typed.** `window.window_handle()`'s
///    `RawWindowHandle::AppKit(handle)` carries `handle.ns_view:
///    NonNull<c_void>`, which by `raw_window_handle`'s own contract points at
///    a live object for as long as the `HasWindowHandle` borrow (`window`,
///    borrowed only for this call's duration and never stored) is valid.
///    Every caller in this crate passes a `gpui::Window` a GPUI callback
///    already holds `&mut` for the callback's own duration, so the pointee
///    cannot have been deallocated underneath this call. `.cast::<NSView>()`
///    is sound because GPUI's own `HasWindowHandle` impl
///    (`vendor/zed-deps/gpui_macos/src/window.rs:1916`, cited in spec §5.4)
///    constructs this exact handle from a pointer whose Objective-C dynamic
///    type already is `NSView*` — the cast does not reinterpret a pointer as
///    a type it was never allocated as. `.as_ref()`'s reference does not
///    escape this function and is never written through, so it cannot alias
///    a `&mut` anyone else holds to the same object.
/// 2. **Every `AppKit` message send below is on the main thread.**
///    `MainThreadMarker::new()` above this block returns `None` — turned into
///    `Err(PinAppearanceError::NotMainThread)` before the block runs — unless
///    this call is already executing on the main thread, so `view.window()`,
///    `view.viewWithTag(...)`, `NSAppearance::appearanceNamed(...)`,
///    `.setAppearance(...)` and `.setNeedsDisplay(...)` (all safe methods
///    `objc2-app-kit` exposes on `NSView`/`NSWindow`/`NSAppearance` — this
///    function's only remaining unsafe is reading the two statics below) can
///    never reach `AppKit` off-main.
/// 3. **The two `extern "C"` statics are valid for the process lifetime.**
///    `NSAppearanceNameAqua`/`NSAppearanceNameDarkAqua` are `AppKit`-provided
///    `&'static NSAppearanceName` constants; `AppKit` guarantees both are
///    initialized before any Cocoa framework call can observe them, and
///    neither is ever deallocated or mutated for the life of the process. A
///    window already exists to pin by the time this function runs, so
///    `AppKit.framework` is already loaded and both statics are already
///    initialized. The value copied out is a `'static` reference, so it
///    cannot dangle after this function returns either.
pub fn pin_appearance(window: impl HasWindowHandle, dark: bool) -> Result<(), PinAppearanceError> {
    let handle = window
        .window_handle()
        .map_err(PinAppearanceError::NoHandle)?;
    let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
        return Err(PinAppearanceError::NotAppKit);
    };
    if MainThreadMarker::new().is_none() {
        return Err(PinAppearanceError::NotMainThread);
    }

    // SAFETY: see this function's doc comment.
    let view: &NSView = unsafe { handle.ns_view.cast::<NSView>().as_ref() };
    // SAFETY: see this function's doc comment.
    let name = unsafe {
        if dark {
            NSAppearanceNameDarkAqua
        } else {
            NSAppearanceNameAqua
        }
    };
    let appearance = NSAppearance::appearanceNamed(name);

    if let Some(blur_view) = view.viewWithTag(NS_VIEW_TAG_BLUR_VIEW) {
        blur_view.setAppearance(appearance.as_deref());
        blur_view.setNeedsDisplay(true);
    } else if let Some(win) = view.window() {
        win.setAppearance(appearance.as_deref());
    }
    view.setNeedsDisplay(true);

    Ok(())
}

/// What [`inspect`] found in `window`'s live `AppKit` state.
///
/// Exists for one reason: a screenshot is the acceptance evidence spec §5.4
/// asks for, and a screen-recording permission this process tree does not
/// hold (`CGPreflightScreenCaptureAccess() == false`, checked live for this
/// item) makes that impossible to produce honestly from here. This is the
/// non-pixel substitute — it reads the same `AppKit` objects [`apply_vibrancy`]
/// and [`pin_appearance`] wrote to, from inside the same process, which needs
/// no screen-recording or accessibility permission at all.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Inspection {
    /// Whether a subview tagged `91_376_254` exists anywhere under `window`'s
    /// view — [`apply_vibrancy`] having actually run and inserted
    /// `window-vibrancy`'s `NSVisualEffectViewTagged`, not merely having been
    /// called without erroring.
    pub blur_view_present: bool,
    /// `window`'s own `NSWindow.isOpaque` — `false` is what makes the blur
    /// view (if present) visible at all: an opaque window paints over
    /// whatever sits behind its content, blur view included.
    pub window_is_opaque: bool,
}

/// Reads back the state [`apply_vibrancy`] and [`pin_appearance`] leave in
/// `window`'s own `AppKit` objects — see [`Inspection`] for why this function
/// exists.
///
/// # Errors
///
/// Same three cases as [`pin_appearance`]: no handle, a non-`AppKit` handle,
/// or not called from the main thread.
///
/// # Safety
///
/// Identical obligation to [`pin_appearance`]'s `# Safety` section, discharged
/// the same way: the single `unsafe` block below performs only the pointer
/// cast that function already proves sound (same `window`, same
/// `HasWindowHandle` contract, same GPUI-constructed `NSView*` handle, same
/// main-thread gate via `MainThreadMarker::new()` above it, same
/// non-escaping, read-only, un-aliased reference). `.window()`,
/// `.viewWithTag(...)` and `.isOpaque()` are all safe `objc2-app-kit` methods,
/// so nothing past the cast needs its own justification.
pub fn inspect(window: impl HasWindowHandle) -> Result<Inspection, PinAppearanceError> {
    let handle = window
        .window_handle()
        .map_err(PinAppearanceError::NoHandle)?;
    let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
        return Err(PinAppearanceError::NotAppKit);
    };
    if MainThreadMarker::new().is_none() {
        return Err(PinAppearanceError::NotMainThread);
    }

    // SAFETY: see this function's doc comment.
    let view: &NSView = unsafe { handle.ns_view.cast::<NSView>().as_ref() };

    let blur_view_present = view.viewWithTag(NS_VIEW_TAG_BLUR_VIEW).is_some();
    // `view.window()` is `None` only if `view` has been detached from any
    // window, which none of this crate's callers do — a conservative `true`
    // (opaque) is the safer default if that assumption is ever wrong, since
    // it reads as "the blur cannot be visible" rather than falsely as "it is."
    let window_is_opaque = match view.window() {
        Some(win) => win.isOpaque(),
        None => true,
    };

    Ok(Inspection {
        blur_view_present,
        window_is_opaque,
    })
}
