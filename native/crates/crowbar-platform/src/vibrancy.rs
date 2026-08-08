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

use std::ffi::c_void;
use std::fmt;
use std::ptr::NonNull;

use objc2::MainThreadMarker;
use objc2_app_kit::{
    NSAppearance, NSAppearanceCustomization, NSAppearanceNameAqua, NSAppearanceNameDarkAqua,
    NSView, NSVisualEffectMaterial, NSVisualEffectState, NSVisualEffectView,
};
use raw_window_handle::{
    AppKitWindowHandle, HandleError, HasWindowHandle, RawWindowHandle, WindowHandle,
};

/// `window-vibrancy` `macos/vibrancy.rs:13`'s own comment: "`NSView::tag` for
/// `NSVisualEffectViewTagged`, just a random number." Not re-exported by the
/// crate (only the `apply_vibrancy`/`clear_vibrancy` functions and the
/// `NSVisualEffectViewTagged` type are public), so the literal is pinned
/// here too — the same way `desktop/src-tauri/src/lib.rs`'s
/// `set_vibrancy_appearance` already pins it, citing the same source line.
const NS_VIEW_TAG_BLUR_VIEW: isize = 91_376_254;

/// Applies the Crowbar-React window's blur material behind `window`: a heavy,
/// smooth `HudWindow` material (`NSVisualEffectMaterial::HUDWindow`) whose
/// vibrancy state follows the window's own active/inactive state
/// (`NSVisualEffectState::FollowsWindowActiveState`), matching
/// `desktop/src-tauri/src/lib.rs`'s `decorate_window` exactly. `window` must
/// already be transparent (`WindowBackgroundAppearance::Transparent`, set at
/// window-creation time) — this only inserts the effect view behind it, it
/// does not make the window transparent itself.
///
/// # The frost has to be a SIBLING of GPUI's view, not its child
///
/// This is the whole reason this function is more than a one-line forward, and
/// it was a shipped bug: **the app rendered a complete sidebar and the window
/// showed nothing but blurred wallpaper.**
///
/// `window-vibrancy` ends with
/// `view.addSubview_positioned_relativeTo(&blurred_view, Below, None)`, where
/// `view` is whatever the raw window handle names. For GPUI that is
/// `AppKitWindowHandle::new(native_view)` — **GPUI's own Metal-backed render
/// view**, not the window's `contentView` (`vendor/zed-deps/gpui_macos/src/
/// window.rs`, which adds `native_view` as a subview of `contentView`). So the
/// effect view was installed as a *child* of the view being rendered into, and
/// on `AppKit` a subview always composites **above** its superview's own layer
/// content. `Below` only orders it against sibling subviews, and GPUI's view
/// has none — so the frost covered the entire UI.
///
/// Walking `view.window().contentView()` makes the effect view a sibling of
/// GPUI's view, which is the arrangement `Below` was written for and the one
/// `desktop/src-tauri` gets for free because Tauri hands Tauri's own content
/// view to the same call.
///
/// S0.5 predicted the hazard in the abstract — *"the handle wraps the
/// `NSView`, not the `NSWindow`, so anything reaching for window-level
/// properties must walk `view.window()` itself"* — and then shipped on
/// [`inspect`] reporting `blur_view_present: true`, which was true and said
/// nothing about **where**. [`Inspection::blur_is_sibling_of_render_view`] is
/// the field that would have caught it, and now does.
///
/// # Safety
///
/// The one `unsafe` here casts the handle's `ns_view` to `&NSView`. It is
/// GPUI-constructed, non-null, and outlives this synchronous main-thread call
/// — the same obligation [`pin_appearance`] discharges, in the same way and
/// for the same handle. The main-thread check above is what makes the
/// subsequent `AppKit` messaging sound.
///
/// # Errors
///
/// `window_vibrancy::Error` — `window` has no `AppKit` raw handle (wrong
/// platform), or `apply_vibrancy` was not called from the main thread, or the
/// running macOS is older than 10.10.
pub fn apply_vibrancy(window: impl HasWindowHandle) -> Result<(), VibrancyError> {
    let handle = window.window_handle().map_err(VibrancyError::NoHandle)?;
    let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
        return Err(VibrancyError::NotAppKit);
    };
    let Some(_main) = MainThreadMarker::new() else {
        return Err(VibrancyError::NotMainThread);
    };

    // SAFETY: see this function's doc comment.
    let view: &NSView = unsafe { handle.ns_view.cast::<NSView>().as_ref() };
    let content_view = view
        .window()
        .and_then(|win| win.contentView())
        .ok_or(VibrancyError::NoContentView)?;

    let raw = NonNull::from(&*content_view).cast::<c_void>();
    let target = ContentView(AppKitWindowHandle::new(raw));

    window_vibrancy::apply_vibrancy(
        target,
        window_vibrancy::NSVisualEffectMaterial::HudWindow,
        Some(window_vibrancy::NSVisualEffectState::FollowsWindowActiveState),
        None,
    )
    .map_err(VibrancyError::Vibrancy)
}

/// What went wrong when [`retune_blur`] could not reach the effect view.
#[derive(Debug)]
pub enum RetuneError {
    /// The window handle could not be borrowed.
    NoHandle(HandleError),
    /// Not an `AppKit` window.
    NotAppKit,
    /// Called off the main thread.
    NotMainThread,
    /// The window has no `contentView`.
    NoContentView,
    /// `contentView` has no `NSVisualEffectView` sibling — the window was not
    /// opened with `WindowBackgroundAppearance::Blurred`, or gpui changed how
    /// it installs the blur.
    NoBlurView,
}

impl core::fmt::Display for RetuneError {
    fn fmt(&self, formatter: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            Self::NoHandle(err) => write!(formatter, "no window handle: {err}"),
            Self::NotAppKit => write!(formatter, "not an AppKit window"),
            Self::NotMainThread => write!(formatter, "not on the main thread"),
            Self::NoContentView => write!(formatter, "window has no contentView"),
            Self::NoBlurView => write!(formatter, "no NSVisualEffectView under contentView"),
        }
    }
}

impl std::error::Error for RetuneError {}

/// Retune **gpui's own** blur view to the React window's material.
///
/// # Why this exists, and why it is not `apply_vibrancy`
///
/// The two apps do not blur the same way, and the difference is visible on
/// screen rather than in any anchor:
///
/// | | material | state |
/// |---|---|---|
/// | `desktop/src-tauri/src/lib.rs` | `HudWindow` | `FollowsWindowActiveState` |
/// | gpui `WindowBackgroundAppearance::Blurred` | **`Selection`** | `Active` |
///
/// `gpui_macos/src/window.rs`'s `BlurredView` hard-codes the second pair. The
/// materials are not interchangeable: measured against the React window over
/// the same desktop, `Selection` let through enough that the window ground
/// read `rgb(64, 73, 82)` where React's read `rgb(49, 57, 67)` — about fifteen
/// levels lighter across the entire chrome, which is exactly the "washed out"
/// difference a screenshot shows and a headless render cannot, because a
/// headless render has no desktop behind it to let through.
///
/// [`apply_vibrancy`] *inserts a second* effect view, which is the wrong
/// shape of fix once gpui already installs one: two stacked effect views
/// composite twice. This reaches the view gpui installed and changes the two
/// properties that differ, leaving its geometry, layer and lifetime to gpui.
///
/// Call it **after** the window is open, on the main thread.
///
/// # Safety
///
/// One `unsafe` construct: `handle.ns_view.cast::<NSView>().as_ref()`, which
/// requires the pointer to be non-null, aligned, and to point at a live
/// `NSView` for the borrow's lifetime — and, because it goes on to send
/// `-subviews`, `-window`, `-contentView`, `-setMaterial:` and `-setState:`,
/// requires the main thread.
///
/// * **Non-null, aligned, live.** `ns_view` comes from a `RawWindowHandle::
///   AppKit` that `raw-window-handle` obtained from the open gpui window; the
///   `WindowHandle` borrow that produced it is still alive on this stack
///   frame, which is precisely the guarantee that type carries. The borrow
///   ends when this function returns, before the window can be closed.
/// * **Class.** `ns_view` is documented by `raw-window-handle` as an `NSView*`,
///   so the cast is to its actual class, not a reinterpretation. The one
///   downcast that could be wrong — to `NSVisualEffectView` — goes through
///   `Retained::downcast`, which is checked at runtime and yields `Err` rather
///   than a mistyped pointer.
/// * **Thread.** `MainThreadMarker::new()` is checked above and returns
///   `NotMainThread` otherwise, so every selector below is sent on the main
///   thread, which is where `AppKit` requires view mutation.
///
/// This is the same obligation [`apply_vibrancy`] discharges, on the same
/// pointer from the same source; only the selectors sent afterwards differ.
///
/// # Errors
///
/// See [`RetuneError`]. Every variant means the material was left alone, so a
/// caller that logs and continues gets gpui's default blur rather than none.
pub fn retune_blur(window: impl HasWindowHandle) -> Result<(), RetuneError> {
    let handle = window.window_handle().map_err(RetuneError::NoHandle)?;
    let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
        return Err(RetuneError::NotAppKit);
    };
    if MainThreadMarker::new().is_none() {
        return Err(RetuneError::NotMainThread);
    }

    // SAFETY: identical to `apply_vibrancy`'s — `ns_view` is a live `NSView`
    // owned by the window gpui opened, borrowed only for this call, and this
    // thread is the main thread (checked above), which is where AppKit
    // requires view mutation to happen.
    let view: &NSView = unsafe { handle.ns_view.cast::<NSView>().as_ref() };
    let content = view
        .window()
        .and_then(|win| win.contentView())
        .ok_or(RetuneError::NoContentView)?;

    // By class, for the reason `inspect` documents: gpui's view carries no
    // `window-vibrancy` tag, so a tag lookup finds nothing on a window that
    // is in fact blurred.
    // `downcast`, not `downcast_ref`: `subviews().iter()` yields owned
    // `Retained<NSView>`s, so a borrow of one cannot outlive the closure.
    let blur = content
        .subviews()
        .iter()
        .find_map(|sub| sub.downcast::<NSVisualEffectView>().ok())
        .ok_or(RetuneError::NoBlurView)?;

    blur.setMaterial(NSVisualEffectMaterial::HUDWindow);
    blur.setState(NSVisualEffectState::FollowsWindowActiveState);
    Ok(())
}

/// A [`HasWindowHandle`] over the window's `contentView`.
///
/// Exists so [`apply_vibrancy`] can hand `window-vibrancy` the **content
/// view** rather than the view GPUI hands out, which is the whole fix — see
/// that function's doc comment.
struct ContentView(AppKitWindowHandle);

impl HasWindowHandle for ContentView {
    /// # Safety
    ///
    /// `WindowHandle::borrow_raw` requires the handle to be valid for the
    /// borrow's lifetime and the window not to be destroyed while it is held.
    /// Both hold here by construction:
    ///
    /// 1. The pointer is taken from a `Retained<NSView>` that
    ///    [`apply_vibrancy`] holds alive across the entire
    ///    `window_vibrancy::apply_vibrancy` call — the only thing that ever
    ///    borrows this handle — so it cannot be deallocated underneath it.
    /// 2. A window's `contentView` is owned by the `NSWindow`, which is owned
    ///    by GPUI and outlives this synchronous, main-thread call. Nothing in
    ///    the call closes a window.
    /// 3. The borrow never escapes: `window_vibrancy::apply_vibrancy` takes
    ///    `impl HasWindowHandle` by value, reads the handle, and returns.
    fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
        // SAFETY: see this method's own doc comment.
        Ok(unsafe { WindowHandle::borrow_raw(RawWindowHandle::AppKit(self.0)) })
    }
}

/// Why [`apply_vibrancy`] could not apply the frost.
#[derive(Debug)]
pub enum VibrancyError {
    /// `window` could not hand back a raw window handle at all.
    NoHandle(HandleError),
    /// `window`'s raw handle exists but is not an `AppKit` handle.
    NotAppKit,
    /// Called off the main thread. `AppKit` view manipulation is main-thread
    /// only.
    NotMainThread,
    /// The view is not in a window, or the window has no content view — so
    /// there is nothing to attach the frost to as a sibling.
    NoContentView,
    /// `window-vibrancy` itself refused.
    Vibrancy(window_vibrancy::Error),
}

impl fmt::Display for VibrancyError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::NoHandle(err) => write!(f, "the window has no raw handle: {err}"),
            Self::NotAppKit => write!(f, "the window's raw handle is not an AppKit handle"),
            Self::NotMainThread => write!(f, "vibrancy must be applied on the main thread"),
            Self::NoContentView => write!(f, "the view is not in a window with a content view"),
            Self::Vibrancy(err) => write!(f, "window-vibrancy refused: {err}"),
        }
    }
}

impl std::error::Error for VibrancyError {}

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
#[derive(Debug, Clone, Copy, PartialEq)]
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
    /// Whether the blur view is a **sibling** of GPUI's render view rather
    /// than its child.
    ///
    /// The field that would have caught S0.5's shipped bug. A blur view
    /// installed as a child of the view being rendered into composites *above*
    /// it — `AppKit` always draws subviews over their superview's own layer —
    /// so the app renders a complete UI and the window shows nothing but
    /// blurred wallpaper. [`blur_view_present`] was `true` throughout.
    ///
    /// [`blur_view_present`]: Inspection::blur_view_present
    pub blur_is_sibling_of_render_view: bool,

    /// The blur view's own frame, `[x, y, w, h]`, or all zeroes when absent.
    ///
    /// A blur view that exists but measures 0x0 renders nothing, and reads in
    /// every other field exactly like one that works.
    pub blur_frame: [f64; 4],
    /// Where the blur sits among its superview's subviews, and how many there
    /// are: `[index, count]`. `[-1, n]` when absent.
    ///
    /// Order is what decides whether the frost is behind the UI or over it,
    /// and it is the one thing no boolean can express.
    pub blur_index_in_superview: [i32; 2],
    /// GPUI's render view's frame, for comparison with the blur's.
    pub render_view_frame: [f64; 4],
    /// The blur view's `material`, as the raw `NSVisualEffectMaterial`.
    ///
    /// Read back from `AppKit` rather than assumed, because
    /// [`retune_blur`] setting it and the window *showing* it are two facts
    /// and only the second one matters. `HudWindow` is 13; gpui's own default,
    /// `Selection`, is 4. `-1` means there was no blur view to ask.
    pub blur_material: isize,
    /// The blur view's `state`. `FollowsWindowActiveState` is 0, `Active` is
    /// 1, `Inactive` is 2. `-1` means there was no blur view to ask.
    pub blur_state: isize,
    /// Whether GPUI's render view is marked opaque. An opaque view over the
    /// blur hides it completely, which looks identical to no blur at all.
    ///
    /// Grouped with [`Self::window_is_opaque`] rather than left as a fourth
    /// bare `bool`, because four of them in one struct is a call site where
    /// every argument is `true, false, true, false` and nobody can read it.
    pub render_view_is_opaque: Opacity,
}

/// Whether a layer paints over what is behind it.
///
/// A named pair rather than a bool: "opaque" and "transparent" are the two
/// things a reader of this report is actually asking about, and a `false`
/// here has been misread once already.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Opacity {
    /// Paints over whatever is behind it — a blur underneath is invisible.
    Opaque,
    /// Lets what is behind it through.
    Transparent,
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

    // By CLASS, not by tag. `window-vibrancy` tags the view it inserts;
    // gpui's own `WindowBackgroundAppearance::Blurred` inserts an
    // `NSVisualEffectView` of its own with no such tag, so a tag lookup
    // reports "no blur" for a window that is in fact blurred — which is a
    // false negative in the one field the chrome is judged by.
    let blur_view_present =
        view.window()
            .and_then(|win| win.contentView())
            .is_some_and(|content| {
                content
                    .subviews()
                    .iter()
                    .any(|sub| sub.downcast_ref::<NSVisualEffectView>().is_some())
            });
    // A blur view found *under GPUI's own view* is the bug: it would paint
    // over everything rendered into that view. Found under the content view
    // and not under GPUI's, it is a sibling, which is the arrangement that
    // works.
    let blur_is_sibling_of_render_view =
        blur_view_present && view.viewWithTag(NS_VIEW_TAG_BLUR_VIEW).is_none();
    // `view.window()` is `None` only if `view` has been detached from any
    // window, which none of this crate's callers do — a conservative `true`
    // (opaque) is the safer default if that assumption is ever wrong, since
    // it reads as "the blur cannot be visible" rather than falsely as "it is."
    let window_is_opaque = match view.window() {
        Some(win) => win.isOpaque(),
        None => true,
    };

    let content = view.window().and_then(|win| win.contentView());
    let blur = content.as_ref().and_then(|content| {
        content
            .subviews()
            .iter()
            .find(|sub| sub.downcast_ref::<NSVisualEffectView>().is_some())
    });

    let (blur_material, blur_state) = blur.as_ref().map_or((-1, -1), |blur| {
        let blur: &NSVisualEffectView = blur
            .downcast_ref::<NSVisualEffectView>()
            .unwrap_or_else(|| unreachable!("`blur` was found by downcasting to this type"));
        (blur.material().0, blur.state().0)
    });

    let blur_frame = blur.as_ref().map_or([0.0; 4], |blur| {
        let frame = blur.frame();
        [
            frame.origin.x,
            frame.origin.y,
            frame.size.width,
            frame.size.height,
        ]
    });

    // Where the blur sits among its siblings. `subviews` is back-to-front, so
    // a lower index is further back — which is what "behind the UI" means.
    // SAFETY: `superview` is `unsafe` in `objc2-app-kit` only because it is
    // main-thread-only — checked above — and returns an autoreleased view
    // owned by the hierarchy, which outlives this synchronous read. Same
    // obligation, same discharge, as every other AppKit call in this module.
    let blur_index_in_superview = blur.as_ref().map_or([-1, 0], |blur| {
        unsafe { blur.superview() }.map_or([-1, 0], |parent| {
            let subviews = parent.subviews();
            let count = i32::try_from(subviews.len()).unwrap_or(i32::MAX);
            let index = subviews
                .iter()
                .position(|sibling| std::ptr::eq(&raw const *sibling, &raw const **blur))
                .and_then(|index| i32::try_from(index).ok())
                .unwrap_or(-1);
            [index, count]
        })
    });

    let render_frame = view.frame();
    let render_view_frame = [
        render_frame.origin.x,
        render_frame.origin.y,
        render_frame.size.width,
        render_frame.size.height,
    ];

    Ok(Inspection {
        blur_view_present,
        window_is_opaque,
        blur_is_sibling_of_render_view,
        blur_frame,
        blur_index_in_superview,
        render_view_frame,
        blur_material,
        blur_state,
        render_view_is_opaque: if view.isOpaque() {
            Opacity::Opaque
        } else {
            Opacity::Transparent
        },
    })
}
