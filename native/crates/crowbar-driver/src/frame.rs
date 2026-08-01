//! **When** a frame may be captured.
//!
//! Everything else in this crate answers "what does this frame contain". This
//! module answers the question that has to be settled first, and that a
//! single-frame capture got wrong: *which* frame.
//!
//! # The defect this exists for
//!
//! `Window::on_next_frame`'s doc comment says "directly after the current frame
//! is rendered", and the implementation delivers on it — a callback runs at the
//! top of the platform's next frame request, which is after the previous draw
//! finished. What it does **not** say, and what a capture has to know, is that
//! one draw is not always the picture:
//!
//! ```text
//! open_window        →  draw 1: the trigger renders; the popup does not exist
//! trigger's prepaint →  trigger_bounds_captured = true
//!                       window.request_animation_frame()
//! frame request 1    →  [our capture]  ← the old code emitted here
//!                       [the vendor's notify]
//!                    →  draw 2: the popup is finally in the tree
//! ```
//!
//! Every deferred, anchored popup in `gpui-component` is built that way: the
//! element that decides where the popup goes reads bounds that only exist once
//! the trigger has been through `prepaint`, so `render` cannot have produced it
//! yet. Capturing after draw 1 recorded a frame the popup was not in, and the
//! run died with `the root anchor "popover-popup" was not recorded this frame`.
//! `select` is the same defect wearing a disguise: its popup *is* in draw 1, but
//! sized off a still-default `Bounds`, so it comes out 2px wide — a wrong number
//! rather than an absent one.
//!
//! # The signal
//!
//! **The capture is taken on the first completed draw that reproduced the
//! previous completed draw's recorded anchors.** Not a frame index, not a delay:
//! a fixpoint over the artefact itself. A snapshot is a pure function of
//! [`RawAnchor`]s, so "the records stopped changing" *is* "the snapshot stopped
//! changing", and nothing else has to be true for that to be the right frame.
//!
//! Two properties make it exactly as cheap as it should be:
//!
//! * A surface that was already right is unaffected. It settles on the second
//!   observation — the second frame request, ~8–16 ms later — because gpui only
//!   redraws a **dirty** window, so a frame request that draws nothing leaves
//!   the records where the previous draw left them. Same records, same bytes.
//! * A surface that needs three draws waits for three, and one that needed
//!   thirty would wait for thirty. Nobody has to know the number, which is the
//!   whole point: the number was never knowable from the call site.
//!
//! It also subsumes the narrower signal it would have been tempting to use.
//! "The window is no longer dirty" is `WindowInvalidator::is_dirty`, which is
//! `pub(crate)` with no accessor — but even reachable it would be **wrong
//! here**, because `spinner` animates for ever and is never not dirty. Its
//! *recorded* geometry is nonetheless constant: gpui rotates at paint time and
//! the extractor reads layout bounds at prepaint, which
//! `row_layout::spinner::the_recorded_box_never_moves_while_it_turns` asserts
//! directly. Comparing records rather than dirtiness is what lets an animated
//! surface settle at all.

use crowbar_ui::gpui::{App, Window};

use crate::element::AnchorRegistry;
use crate::record::RawAnchor;

/// How many consecutive **changing** draws are taken as proof that a surface has
/// no settled frame at all.
///
/// **This is not a timeout and it cannot make a capture happen.** It changes no
/// outcome except that a surface which would otherwise spin for ever gets
/// reported instead: the capture is triggered by [`Observation::Settled`] and by
/// nothing else, and a surface that settles does so on its second observation
/// whatever this number is. Any value at all would detect the condition; a large
/// one only means the refusal takes longer to arrive.
///
/// The condition it detects is a surface whose *anchored* geometry is a function
/// of time. Such a surface has no single-frame picture, so there is nothing for
/// a v1 snapshot to be — which makes a loud refusal the correct outcome and a
/// hang the incorrect one.
pub const UNSETTLED_FRAME_LIMIT: usize = 300;

/// What one look at the recorded frame found.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Observation {
    /// This draw recorded exactly what the draw before it did. The picture has
    /// stopped changing, and this is the frame to capture.
    Settled,
    /// It recorded something else. Another draw is coming; look again after it.
    Changing,
    /// It has recorded something else [`UNSETTLED_FRAME_LIMIT`] times running.
    /// This surface has no settled frame — see that constant.
    NeverSettled,
}

/// Watches successive completed draws and says when the recorded frame has
/// stopped changing.
///
/// Held across frames by whatever is delivering them: the binary's platform
/// frame loop through [`on_settled_frame`], and the layout harness's
/// `simulate_next_frame` pump, which is the same signal read by the one loop
/// gpui gives a test — *"tests have no platform frame loop"*.
pub struct Settling {
    registry: AnchorRegistry,
    /// What the previous observation found, or `None` before the first.
    previous: Option<Vec<RawAnchor>>,
    /// Consecutive [`Observation::Changing`]s, for [`UNSETTLED_FRAME_LIMIT`].
    changed: usize,
}

impl Settling {
    /// Starts watching `registry`. Cheap: the registry is a shared handle.
    #[must_use]
    pub fn watching(registry: &AnchorRegistry) -> Self {
        Self {
            registry: registry.clone(),
            previous: None,
            changed: 0,
        }
    }

    /// Looks at the frame that has just been drawn.
    ///
    /// Call this **after** a draw has completed and not before — from a
    /// `Window::on_next_frame` callback, or after a `simulate_next_frame` and
    /// the `run_until_parked` that lets the redraw it triggered happen. Called
    /// before any draw it would compare an empty registry with itself and report
    /// a settled frame that nothing had painted.
    pub fn observe(&mut self) -> Observation {
        let current = self.registry.records();
        if self.previous.as_ref() == Some(&current) {
            return Observation::Settled;
        }
        self.previous = Some(current);
        self.changed += 1;
        if self.changed >= UNSETTLED_FRAME_LIMIT {
            return Observation::NeverSettled;
        }
        Observation::Changing
    }
}

/// Runs `capture` once, on the first frame whose recorded anchors match the
/// frame before it.
///
/// The window's own frame loop is what delivers the draws, so this is the form
/// the **binary** uses; a test has no such loop and drives [`Settling`] itself.
///
/// `capture` is handed the [`Observation`] that ended the wait, which is
/// [`Observation::Settled`] or [`Observation::NeverSettled`] —
/// [`Observation::Changing`] is what re-arms, so it can never reach a caller.
/// It is passed rather than assumed so that a refusal is the caller's to word:
/// the driver knows the frame never settled, and only the caller knows what it
/// was trying to write.
pub fn on_settled_frame(
    window: &Window,
    registry: &AnchorRegistry,
    capture: impl FnOnce(Observation, &mut Window, &mut App) + 'static,
) {
    arm(window, Settling::watching(registry), Box::new(capture));
}

/// [`on_settled_frame`]'s callback, boxed so that [`arm`] can hand the same one
/// to the next frame without being generic over a type it would have to
/// instantiate for ever.
type Capture = Box<dyn FnOnce(Observation, &mut Window, &mut App)>;

/// One leg of [`on_settled_frame`], which re-arms itself until the picture
/// stops moving.
fn arm(window: &Window, mut settling: Settling, capture: Capture) {
    window.on_next_frame(move |window, cx| match settling.observe() {
        Observation::Changing => arm(window, settling, capture),
        settled => capture(settled, window, cx),
    });
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{
        Context, IntoElement, ParentElement as _, Render, Styled as _, TestAppContext, Window,
        canvas, div, px, size,
    };

    use super::{Observation, Settling, on_settled_frame};
    use crate::element::{AnchorRegistry, anchor, anchor_root, install};

    /// A surface whose root anchor **does not exist on the first draw**, built
    /// out of exactly what `gpui_component::Popover` is built out of.
    ///
    /// The vendor's popup is behind `if !open || !trigger_bounds_captured`, and
    /// `trigger_bounds_captured` is set from the trigger's `on_prepaint` — which
    /// `gpui-component` implements as an absolutely-positioned `canvas` child,
    /// so its callback runs during `prepaint`, after `render` has already
    /// returned. On the first capture it asks for another frame. That is
    /// reproduced here rather than imported, because `crowbar-driver` must not
    /// depend on `gpui-component` (§4.2) and because a probe that stopped
    /// matching the vendor would be a test of nothing.
    struct DeferredSurface {
        measured: Rc<Cell<bool>>,
    }

    impl Render for DeferredSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            let measured = self.measured.clone();
            let trigger = div().w(px(120.0)).h(px(24.0)).child(
                canvas(
                    move |_bounds, window, _cx| {
                        if !measured.get() {
                            measured.set(true);
                            window.request_animation_frame();
                        }
                    },
                    |_, (), _, _| {},
                )
                .absolute()
                .size_full(),
            );

            if !self.measured.get() {
                return div().child(trigger).into_any_element();
            }
            anchor_root(
                "popup",
                div()
                    .w(px(200.0))
                    .h(px(80.0))
                    .child(trigger)
                    .child(anchor("popup-viewport", div().w(px(198.0)).h(px(78.0)))),
            )
            .into_any_element()
        }
    }

    fn a_deferred_window(
        cx: &mut TestAppContext,
    ) -> (AnchorRegistry, gpui::WindowHandle<DeferredSurface>) {
        let anchors: AnchorRegistry = cx.update(install);
        let window = cx.open_window(size(px(400.0), px(300.0)), |_, _| DeferredSurface {
            measured: Rc::new(Cell::new(false)),
        });
        cx.run_until_parked();
        (anchors, window)
    }

    /// **The frame a single observation would have captured is empty.**
    ///
    /// This is the defect stated as an assertion, and it is the control the
    /// tests below need: without it, "the settled frame has the popup" would
    /// also pass on a build that captured the first frame, because nobody would
    /// have said the first frame was different.
    #[gpui::test]
    fn the_first_drawn_frame_records_no_anchor_at_all(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let (anchors, _window) = a_deferred_window(cx);

        assert!(
            anchors.records().is_empty(),
            "the deferred popup is not in the first frame; got {:?}",
            anchors
                .records()
                .iter()
                .map(|r| r.id.clone())
                .collect::<Vec<_>>(),
        );
    }

    /// **The settled frame is the one with the popup in it**, and it is reached
    /// by looking at what was drawn rather than by counting to two.
    #[gpui::test]
    fn settling_waits_for_the_draw_the_popup_is_in(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let (anchors, window) = a_deferred_window(cx);

        let mut settling = Settling::watching(&anchors);
        let mut looks = 0;
        loop {
            looks += 1;
            match settling.observe() {
                Observation::Settled => break,
                Observation::Changing => {}
                Observation::NeverSettled => panic!("the probe never settled"),
            }
            window
                .update(cx, |_, window, cx| window.simulate_next_frame(cx))
                .expect("the window is open");
            cx.run_until_parked();
        }

        let ids: Vec<String> = anchors
            .records()
            .iter()
            .map(|record| record.id.to_string())
            .collect();
        assert_eq!(ids, ["popup", "popup-viewport"]);
        // Three looks: the empty first draw, the draw that added the popup, and
        // the one that reproduced it. Asserted so that a change which settled
        // *earlier* — on the empty frame — fails here rather than silently
        // capturing nothing.
        assert_eq!(looks, 3);
    }

    /// A surface that was already right settles on the **second** look, with no
    /// draw in between: gpui redraws only a dirty window, so an unchanged
    /// picture is an unchanged record set.
    #[gpui::test]
    fn a_static_surface_settles_on_the_second_look(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors: AnchorRegistry = cx.update(install);
        let _window = cx.open_window(size(px(400.0), px(300.0)), |_, _| StaticSurface);
        cx.run_until_parked();

        let mut settling = Settling::watching(&anchors);
        assert_eq!(settling.observe(), Observation::Changing);
        assert_eq!(settling.observe(), Observation::Settled);
    }

    struct StaticSurface;

    impl Render for StaticSurface {
        fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            anchor_root("root", div().w(px(100.0)).h(px(20.0)))
        }
    }

    /// [`on_settled_frame`] — the form the **binary** arms — fires once, and
    /// fires on the frame the popup is in.
    ///
    /// The callback is registered from inside `open_window`'s builder, exactly
    /// as `main.rs` registers it, so the ordering this depends on is the
    /// binary's: the capture is queued before the first draw and the vendor's
    /// `request_animation_frame` is queued during it.
    #[gpui::test]
    fn the_armed_capture_runs_once_on_the_settled_frame(cx: &mut TestAppContext) {
        crate::leak_checked!(cx);
        let anchors: AnchorRegistry = cx.update(install);
        let captured: Rc<Cell<usize>> = Rc::new(Cell::new(0));
        let seen: Rc<std::cell::RefCell<Vec<String>>> =
            Rc::new(std::cell::RefCell::new(Vec::new()));

        let window = {
            let anchors = anchors.clone();
            let captured = captured.clone();
            let seen = seen.clone();
            cx.open_window(size(px(400.0), px(300.0)), move |window, _| {
                let records = anchors.clone();
                on_settled_frame(window, &anchors, move |observation, _window, _cx| {
                    assert_eq!(observation, Observation::Settled);
                    captured.set(captured.get() + 1);
                    *seen.borrow_mut() = records
                        .records()
                        .iter()
                        .map(|record| record.id.to_string())
                        .collect();
                });
                DeferredSurface {
                    measured: Rc::new(Cell::new(false)),
                }
            })
        };
        cx.run_until_parked();

        for _ in 0..8 {
            window
                .update(cx, |_, window, cx| window.simulate_next_frame(cx))
                .expect("the window is open");
            cx.run_until_parked();
        }

        assert_eq!(captured.get(), 1, "the capture must run exactly once");
        assert_eq!(*seen.borrow(), ["popup", "popup-viewport"]);
    }
}
