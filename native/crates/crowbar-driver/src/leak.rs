//! `leak_checked!` — the leak assertion every `#[gpui::test]` arms (§17).
//!
//! # Why an explicit assertion, when gpui already has a teardown check
//!
//! `#[gpui::test]` expands to a harness that holds
//! `App::ref_counts_drop_handle()` for the duration of the test, and
//! `LeakDetector`'s `Drop` panics if any entity handle is still alive when that
//! `Arc` reaches zero (`vendor/gpui/src/app/entity_map.rs`). `test-support`
//! turns `leak-detection` on, and `TestAppContext` does not exist without
//! `test-support`, so that check is compiled into every test here already.
//!
//! It is also **defeatable, silently.** The `Arc` it hangs off lives inside
//! `App`, so anything that keeps the app alive past the harness keeps the
//! detector alive too, and a `Drop` that never runs reports nothing. A probe
//! that leaked an entity *and* a `TestAppContext` clone passed green:
//!
//! ```ignore
//! let entity = cx.update(|cx| cx.new(|_| Thing));
//! std::mem::forget(entity);        // caught on its own
//! std::mem::forget(cx.clone());    // …and not caught with this line added
//! ```
//!
//! `App::assert_no_new_leaks` has no such dependency: it reads the detector's
//! live state at the moment it is called. That is the check this macro arms,
//! and `a_leaked_entity_is_reported_even_when_the_app_outlives_the_test` in
//! `element.rs` is the control that keeps the claim honest.
//!
//! # Ordering
//!
//! `assert_no_new_leaks` reports every entity created since the snapshot that
//! still holds a handle — an *open window's* root view included, because from
//! the detector's side an entity that is still alive and one that has leaked
//! look the same. So entities have to be released before the detector runs;
//! `headless_app_context.rs:189-195` says the same thing about its own `Drop`.
//! The guard therefore parks, quits (which clears the window map and flushes
//! the effects that release the root views), parks again, and only then
//! asserts.
//!
//! # Why a macro rather than a helper function
//!
//! Two reasons, both about what the shape makes impossible.
//!
//! * The macro names the binding, so the one misuse that would compile and do
//!   nothing — `let _ = leak_guard(cx);`, a temporary dropped on the spot,
//!   before the window it is supposed to outlive — cannot be written.
//! * A macro body is type-checked at the expansion site, so this needs no
//!   `test-support` feature plumbed from `crowbar-driver` down to a `gpui`
//!   that is only a dev-dependency. `crowbar-app`'s tests expand it against
//!   their own `gpui`.
//!
//! It is still a call each test makes, which is the design that rots, so it is
//! gated: `scripts/check-invariants.sh` rule 6 fails on any `#[gpui::test]`
//! whose first statement is not this macro.

/// Arms gpui's leak detector for the remainder of the enclosing test.
///
/// Call it as the **first statement** of a `#[gpui::test]` — Rust drops locals
/// in reverse declaration order, so the guard declared first is the one that
/// runs last, after every window handle and entity the test bound.
///
/// ```ignore
/// #[gpui::test]
/// fn a_thing_holds(cx: &mut TestAppContext) {
///     crowbar_driver::leak_checked!(cx);
///     …
/// }
/// ```
///
/// Requires `gpui` with `test-support` in scope at the expansion site, which is
/// exactly the condition for `#[gpui::test]` to compile at all.
#[macro_export]
macro_rules! leak_checked {
    ($cx:expr) => {
        let __crowbar_leak_guard = {
            struct LeakGuard {
                cx: ::gpui::TestAppContext,
                snapshot: ::gpui::LeakDetectorSnapshot,
            }

            impl ::core::ops::Drop for LeakGuard {
                fn drop(&mut self) {
                    // A failing assertion is already unwinding through here;
                    // panicking a second time aborts the process and takes the
                    // real failure's message with it. gpui's own detector
                    // declines for the same reason.
                    if ::std::thread::panicking() {
                        return;
                    }
                    self.cx.run_until_parked();
                    self.cx.quit();
                    self.cx.run_until_parked();
                    let snapshot = &self.snapshot;
                    self.cx.update(move |app| app.assert_no_new_leaks(snapshot));
                }
            }

            let cx: ::gpui::TestAppContext = ::core::clone::Clone::clone($cx);
            let snapshot = cx.update(|app| app.leak_detector_snapshot());
            LeakGuard { cx, snapshot }
        };
    };
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    struct Probe;

    /// The control for the claim the module comment makes, and the reason the
    /// explicit assertion is worth its 53 call sites: a leak that gpui's
    /// teardown `Drop` check reports on its own goes **unreported** the moment
    /// something outlives the harness and keeps the detector alive with it.
    ///
    /// It is run against a throwaway app so that the leak it deliberately makes
    /// is not this test's own, and it asserts in both directions — that the
    /// assertion fires on the leak, and that it is quiet without one. An
    /// `assert_no_new_leaks` that had been turned into a no-op would fail the
    /// first of those, which is what keeps the other 52 sites from being a
    /// green light over nothing.
    #[gpui::test]
    fn the_assertion_catches_a_leak_the_teardown_check_misses(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);

        let probe = cx.new_app();
        let snapshot = probe.update(|cx| cx.leak_detector_snapshot());

        probe.update(|cx| cx.assert_no_new_leaks(&snapshot));

        let entity = probe.update(|cx| cx.new(|_| Probe));
        std::mem::forget(entity);

        let caught = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            probe.update(|cx| cx.assert_no_new_leaks(&snapshot));
        }));
        assert!(
            caught.is_err(),
            "a leaked entity created after the snapshot must be reported",
        );

        // Keeping the app alive past the end of the test is exactly what
        // silences gpui's own `Drop`-based check — done deliberately here, both
        // to reproduce the hole and so the leak above does not reach this
        // test's own guard.
        std::mem::forget(probe);
    }

    /// A failing test must fail with **its own** message. The guard runs from a
    /// `Drop`, so if it asserted while the thread was already unwinding the
    /// second panic would abort the process and take the real failure's message
    /// with it — the one moment a developer needs it most.
    ///
    /// The inner block leaks deliberately, so this is not a test of the quiet
    /// path: with the `thread::panicking()` check removed it is an abort, which
    /// is loud but is *not* a reported failure. That is the trade this check
    /// makes and the reason it is spelled out here.
    #[gpui::test]
    fn the_guard_stands_down_while_a_test_is_already_failing(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);

        let probe = cx.new_app();
        let failure = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            crate::leak_checked!(&probe);
            let leaked = probe.update(|cx| cx.new(|_| Probe));
            std::mem::forget(leaked);
            panic!("the test's own failure");
        }));

        let payload = failure.expect_err("the block panics");
        assert_eq!(
            payload.downcast_ref::<&str>().copied(),
            Some("the test's own failure"),
            "the guard replaced the failure it was supposed to leave alone",
        );

        std::mem::forget(probe);
    }
}
