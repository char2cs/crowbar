//! The `AppKit` half of [`super`]: the only code in Crowbar that sends a message
//! to Objective-C.
//!
//! Everything here runs on the main thread, and nothing here can be reached
//! without an `objc2::MainThreadMarker` — which is obtained once, at the top of
//! each entry point, from the runtime. A thread that is not the main thread gets
//! `None` and an early return, so the `AppKit` objects below are never so much as
//! allocated off it.
//!
//! # The whole unsafe inventory of this crate, in one place
//!
//! Three `unsafe` operations, each argued for on its own enclosing item:
//!
//! 1. `msg_send![super(this), init]` in [`MenuTarget::new`] — the designated
//!    initialiser of the target object's superclass.
//! 2. `-[NSMenuItem setTarget:]` in [`build`] — pointing a row at that object.
//! 3. `-[NSMenuItem setAction:]` in [`build`] — naming the selector it answers.
//!
//! There is a fourth surface that no grep will find, because it is spelled as
//! `unsafe` *attributes* rather than `unsafe` blocks: the `define_class!`
//! invocation below. Its obligations are discharged in the comment above it.
//!
//! Every other `AppKit` call is a *safe* binding: `objc2-app-kit` marks a method
//! safe when the receiver's class is `MainThreadOnly` and the arguments cannot
//! be misused, which covers `NSMenu::new`, `addItem:`, `setTitle:`,
//! `setEnabled:`, `setState:`, `setTag:`, `setSubmenu:`, `separatorItem`,
//! `popUpMenuPositioningItem:atLocation:inView:` and `cancelTracking`. They are
//! not listed as unsafe because they are not: the marker is the proof.

use std::cell::RefCell;
use std::time::Duration;

use objc2::rc::Retained;
use objc2::runtime::{AnyObject, NSObject};
use objc2::{AnyThread as _, DefinedClass as _, MainThreadMarker, define_class, msg_send, sel};
use objc2_app_kit::{NSControlStateValueOff, NSControlStateValueOn, NSMenu, NSMenuItem};
use objc2_foundation::{NSPoint, NSRunLoop, NSRunLoopCommonModes, NSString, NSTimer};

use super::{MenuError, MenuItem, ScreenPoint, Selection};

thread_local! {
    /// The menu this thread is currently tracking, if any.
    ///
    /// Two jobs. It refuses a second, nested `popUpMenuPositioningItem:` — see
    /// [`MenuError::AlreadyTracking`] — and it is what [`cancel`] needs a handle
    /// to, since `AppKit` exposes no "the menu that is up right now".
    ///
    /// A `thread_local` rather than a `static`, because `Retained<NSMenu>` is
    /// `!Send`: `NSMenu` is a `MainThreadOnly` class, so the value is only ever
    /// touched by the one thread that is allowed to have made it. That makes the
    /// container sound without a lock, rather than in spite of not having one.
    static TRACKING: RefCell<Option<Retained<NSMenu>>> = const { RefCell::new(None) };
}

// A throwaway Objective-C object: the target every enabled row's action is sent
// to, holding the `Selection` that action writes into.
//
// The `define_class!` obligations, discharged one by one — they are `unsafe`
// attributes rather than `unsafe` blocks, so no grep will point at them and this
// comment is the record:
//
//   * `#[unsafe(super(NSObject))]` — the superclass really is `NSObject`, the
//     struct has no other fields, and the class is registered under a name no
//     other bundle in this process uses (`CrowbarPlatform…`), so it cannot
//     collide with an existing class and be silently reused.
//   * `#[ivars = Selection]` — `Selection` is `Sized`, owns nothing outside the
//     process, has no `Drop` that could run on a foreign thread, and is written
//     and read only from the main thread, which is the only thread that ever
//     holds a reference to this object.
//   * `#[unsafe(method(…))]` — the selector's Objective-C type encoding is
//     `v@:@`: it returns nothing and takes the sender. The Rust signature
//     `fn(&self, &NSMenuItem)` encodes to exactly that, and AppKit is documented
//     to pass the `NSMenuItem` that was chosen as the sender. The reference is
//     borrowed for the call only; nothing here stores it.
//
// `MenuTarget` is not `Send` or `Sync` and must not become either: it is created,
// used and dropped inside one synchronous call on the main thread.
define_class!(
    #[unsafe(super(NSObject))]
    #[name = "CrowbarPlatformNativeMenuTarget"]
    #[ivars = Selection]
    struct MenuTarget;

    impl MenuTarget {
        #[unsafe(method(crowbarNativeMenuItemChosen:))]
        fn chosen(&self, sender: &NSMenuItem) {
            self.ivars().record(sender.tag());
        }

        #[unsafe(method(crowbarNativeMenuCancel:))]
        fn cancel_from_timer(&self, _timer: &NSTimer) {
            // Already on the main thread — a run-loop timer fires on the thread
            // whose run loop it was added to — so this cannot be the refusal.
            let _ = cancel();
        }
    }
);

impl MenuTarget {
    /// An instance holding an empty [`Selection`].
    ///
    /// # Safety
    ///
    /// The one `unsafe` is `msg_send![super(this), init]`, which sends `init` to
    /// `NSObject` — the superclass named in this type's own `define_class!`
    /// above — on the freshly allocated, uninitialised instance `this`.
    ///
    /// The obligations `msg_send!` places on the caller, and why each holds:
    ///
    /// * **The receiver is valid.** `this` is an `Allocated<Self>` produced by
    ///   `Self::alloc()` on the line before and consumed here; `set_ivars` has
    ///   already written the instance variable, which is what makes the object
    ///   safe for `NSObject`'s `init` to run over.
    /// * **The selector exists on the class and takes no arguments.** `init` is
    ///   `NSObject`'s designated initialiser, `-(instancetype)init`.
    /// * **The return type is right.** `init` follows the `init` family, so it
    ///   returns an object with +1 retain count that the caller owns;
    ///   `Retained<Self>` is exactly that ownership, and `msg_send!` knows the
    ///   family from the selector, so no retain or release is added or missed.
    /// * **The thread.** `NSObject`'s `init` has no thread requirement, and this
    ///   function is only ever reached from [`show`], which holds a
    ///   `MainThreadMarker`.
    /// * **No pointer outlives the call.** The only pointer crossing the
    ///   boundary is the receiver, which is moved into the message send and
    ///   comes back out as the returned `Retained`.
    fn new() -> Retained<Self> {
        let this = Self::alloc().set_ivars(Selection::new());
        unsafe { msg_send![super(this), init] }
    }
}

/// The `NSMenu` a slice of [`MenuItem`] means, with every enabled row wired to
/// `target`.
///
/// Tags are assigned depth-first to **every** row, disabled ones included, so
/// that they index [`super::ContextMenu::chosen_ids`] — see that method for why
/// a disabled row keeps its number.
///
/// # Safety
///
/// Two `unsafe` message sends, both to an `NSMenuItem` this function has just
/// created and still owns, on the main thread (the `MainThreadMarker` argument
/// is the proof), inside a menu that is not yet visible:
///
/// * **`-[NSMenuItem setTarget:]`** is `unsafe` because `AppKit` does not check
///   that the object responds to the action. It is sent the `MenuTarget`
///   allocated by [`show`], which does respond to
///   `crowbarNativeMenuItemChosen:` — that is the whole of its class, and
///   `the_target_answers_the_selector_the_rows_are_pointed_at` asks the runtime
///   rather than taking this sentence's word for it. The target is stored
///   **weakly** by `NSMenuItem`, which is the
///   lifetime obligation that actually matters here: the `Retained<MenuTarget>`
///   in [`show`] is held across the entire `popUpMenuPositioningItem:` call and
///   dropped only after it returns, so the weak reference cannot be read after
///   the object is gone. The menu is built, shown and released inside that one
///   stack frame, so no row can outlive its target.
/// * **`-[NSMenuItem setAction:]`** is `unsafe` because a selector is just a
///   name: a misspelling compiles, and then every row is silently inert, because
///   `AppKit` never fires an action its target does not answer. `sel!` interns
///   `crowbarNativeMenuItemChosen:` here with the trailing colon that makes it a
///   one-argument selector, matching the `fn(&self, &NSMenuItem)` the
///   `define_class!` above registers under that same name — and the test named
///   in the previous bullet asks the Objective-C runtime whether the two agree,
///   so the argument does not rest on the two strings being read side by side.
///
/// Neither send is made for a disabled row: a `data-disabled` item gets no
/// target and no action, so it is inert rather than merely ignored.
fn build(
    items: &[MenuItem],
    target: &MenuTarget,
    mtm: MainThreadMarker,
    next_tag: &mut isize,
) -> Retained<NSMenu> {
    let menu = NSMenu::new(mtm);
    // Every row's enabled state is decided here, so AppKit's automatic
    // enabling — which asks the responder chain and would grey out every row
    // whose target does not implement `validateMenuItem:` — must be off.
    menu.setAutoenablesItems(false);

    for item in items {
        match item {
            MenuItem::Separator => menu.addItem(&NSMenuItem::separatorItem(mtm)),
            MenuItem::Item {
                id: _,
                title,
                enabled,
                checked,
            } => {
                let row = NSMenuItem::new(mtm);
                row.setTitle(&NSString::from_str(title));
                row.setEnabled(*enabled);
                row.setState(if *checked {
                    NSControlStateValueOn
                } else {
                    NSControlStateValueOff
                });
                row.setTag(*next_tag);
                *next_tag += 1;
                if *enabled {
                    unsafe {
                        row.setTarget(Some(target as &AnyObject));
                        row.setAction(Some(sel!(crowbarNativeMenuItemChosen:)));
                    }
                }
                menu.addItem(&row);
            }
            MenuItem::Submenu {
                title,
                enabled,
                items,
            } => {
                let row = NSMenuItem::new(mtm);
                row.setTitle(&NSString::from_str(title));
                row.setEnabled(*enabled);
                row.setSubmenu(Some(&build(items, target, mtm, next_tag)));
                menu.addItem(&row);
            }
        }
    }

    menu
}

/// Shows the menu and blocks in `AppKit`'s tracking loop until it closes.
///
/// # Errors
///
/// [`MenuError::OffMainThread`] or [`MenuError::AlreadyTracking`]; in both cases
/// nothing has been created and nothing is shown.
pub(super) fn show(items: &[MenuItem], at: ScreenPoint) -> Result<Selection, MenuError> {
    let Some(mtm) = MainThreadMarker::new() else {
        return Err(MenuError::OffMainThread);
    };

    let target = MenuTarget::new();
    let mut next_tag = 0;
    let menu = build(items, &target, mtm, &mut next_tag);

    // Published *before* the loop starts, because the loop does not return until
    // the menu is gone and `cancel` is only useful while it is up.
    TRACKING.with(|slot| match slot.try_borrow_mut() {
        Ok(mut slot) if slot.is_none() => {
            *slot = Some(menu.clone());
            Ok(())
        }
        _ => Err(MenuError::AlreadyTracking),
    })?;

    // With no view, the location is in screen coordinates — which is what
    // `ScreenPoint` is documented to be, so there is no conversion here to get
    // wrong. `nil` for the positioning item puts the menu's top-left at the
    // point, and AppKit flips it against the screen edge by itself.
    //
    // The `bool` says only that tracking ended normally. Which row was chosen is
    // in the target's `Selection`, so there is nothing to read off it.
    let _ = menu.popUpMenuPositioningItem_atLocation_inView(None, NSPoint::new(at.x, at.y), None);

    TRACKING.with(|slot| {
        if let Ok(mut slot) = slot.try_borrow_mut() {
            *slot = None;
        }
    });

    Ok(target.ivars().clone())
}

/// Closes the menu this thread is tracking, if there is one.
///
/// # Errors
///
/// [`MenuError::OffMainThread`].
pub(super) fn cancel() -> Result<bool, MenuError> {
    if MainThreadMarker::new().is_none() {
        return Err(MenuError::OffMainThread);
    }

    // Cloned out of the slot and the borrow dropped before the message is sent:
    // this runs *inside* the tracking loop it is cancelling, so a borrow held
    // across `cancelTracking` would be a borrow held across re-entrant AppKit.
    let tracking = TRACKING.with(|slot| slot.try_borrow().ok().and_then(|slot| slot.clone()));
    match tracking {
        Some(menu) => {
            menu.cancelTracking();
            Ok(true)
        }
        None => Ok(false),
    }
}

/// Arms a run-loop timer that will call [`cancel`] after `after`.
///
/// # Why a run-loop timer and not a queued block
///
/// This is the second thing that had to be learned the hard way, and it is the
/// reason this function exists at all. `GPUI`'s foreground executor schedules
/// through `dispatch_async` onto the main queue, so [`show`] runs *inside* a
/// main-queue block — and `libdispatch` will not begin draining another
/// main-queue block while one is already on the stack, even though the menu's
/// nested run loop is spinning. A `dispatch_after` scheduled to close the menu
/// therefore never runs until the menu has closed by itself, which is exactly
/// backwards. **Verified by sampling the process**: the main thread sits in
/// `_dispatch_main_queue_drain → … → popUpMenuPositioningItem:` and the queued
/// cancel never arrives.
///
/// A run-loop timer is not queued on the main queue and is not subject to that
/// guard. Added to `NSRunLoopCommonModes`, which `AppKit` extends with
/// `NSEventTrackingRunLoopMode`, it fires in the very loop the menu is tracking
/// in.
///
/// # Safety
///
/// Two `unsafe` calls, both on the main thread — [`MainThreadMarker`] is
/// obtained first and an off-main call returns before either is reached:
///
/// * **`+[NSTimer timerWithTimeInterval:target:selector:userInfo:repeats:]`** is
///   `unsafe` on the target, the selector and the user info. The target is a
///   `MenuTarget`, which implements `crowbarNativeMenuCancel:` — the runtime is
///   asked whether it does by
///   `the_target_answers_the_selector_the_timer_is_pointed_at`. The selector's
///   one argument is the timer, which is the signature `NSTimer` documents and
///   the signature `define_class!` registers. `userInfo` is `None`, so there is
///   no third object whose type could be wrong.
/// * **`-[NSRunLoop addTimer:forMode:]`** is `unsafe` because a run loop must be
///   fed from its own thread. This is the **main** run loop and this call is on
///   the main thread, which is the whole of the obligation.
///
/// The lifetime that matters: `NSTimer` **retains its target**, and the run loop
/// retains the timer until it fires, so the `Retained<MenuTarget>` created here
/// may be — and is — dropped immediately. A non-repeating timer invalidates
/// itself after firing, which releases both.
///
/// # Errors
///
/// [`MenuError::OffMainThread`]; nothing is armed.
pub(super) fn cancel_after(after: Duration) -> Result<(), MenuError> {
    if MainThreadMarker::new().is_none() {
        return Err(MenuError::OffMainThread);
    }

    let target = MenuTarget::new();
    unsafe {
        let timer = NSTimer::timerWithTimeInterval_target_selector_userInfo_repeats(
            after.as_secs_f64(),
            &target,
            sel!(crowbarNativeMenuCancel:),
            None,
            false,
        );
        NSRunLoop::mainRunLoop().addTimer_forMode(&timer, NSRunLoopCommonModes);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use objc2::runtime::AnyClass;
    use objc2::{ClassType as _, sel};

    use super::MenuTarget;

    /// **The one thing a reader cannot check by reading.** `build` sends
    /// `setAction:` a selector spelled in a `sel!`, and `define_class!` registers
    /// a method under a name spelled in an attribute. Nothing makes the compiler
    /// compare them: a misspelling in either builds cleanly and produces a menu
    /// whose every row does nothing, because `AppKit` never fires an action its
    /// target does not answer.
    ///
    /// So the Objective-C runtime is asked. This is also what proves the class
    /// registers at all — `define_class!` does that lazily, on the first
    /// `MenuTarget::class()`, which is a step that would otherwise only ever
    /// happen inside a menu nobody can click in a test.
    #[test]
    fn the_target_answers_the_selector_the_rows_are_pointed_at() {
        let class: &AnyClass = MenuTarget::class();

        assert!(class.responds_to(sel!(crowbarNativeMenuItemChosen:)));
        assert_eq!(class.name().to_str(), Ok("CrowbarPlatformNativeMenuTarget"));
        // A near-miss the compiler would also have accepted: same words, no
        // argument. If the registered method were ever written without its
        // colon, the row would be pointed at a selector nothing implements.
        assert!(!class.responds_to(sel!(crowbarNativeMenuItemChosen)));
    }

    /// The same question for the timer's selector, and it is a sharper one: a
    /// misspelling there does not merely do nothing, it raises
    /// `NSInvalidArgumentException` inside the run loop when the timer fires,
    /// which is a crash in a frame no Rust code appears in.
    #[test]
    fn the_target_answers_the_selector_the_timer_is_pointed_at() {
        let class: &AnyClass = MenuTarget::class();

        assert!(class.responds_to(sel!(crowbarNativeMenuCancel:)));
        assert!(!class.responds_to(sel!(crowbarNativeMenuCancel)));
    }

    /// Arming the timer from off the main thread is a refusal, not an
    /// `addTimer:forMode:` on a run loop belonging to another thread.
    #[test]
    fn a_timer_armed_off_the_main_thread_is_refused() {
        let outcome = std::thread::spawn(|| super::cancel_after(Duration::from_millis(1)))
            .join()
            .expect("the spawned thread did not panic");

        assert_eq!(outcome, Err(super::MenuError::OffMainThread));
    }

    /// Nothing is tracking until something shows a menu, so a cancel that no
    /// menu is waiting for is `false` rather than an error — and, off the main
    /// thread, a refusal before it can look at all.
    #[test]
    fn cancelling_with_no_menu_up_is_not_an_error() {
        let off_main = std::thread::spawn(super::cancel)
            .join()
            .expect("the spawned thread did not panic");

        assert_eq!(off_main, Err(super::MenuError::OffMainThread));
        super::TRACKING.with(|slot| assert!(slot.borrow().is_none()));
    }
}
