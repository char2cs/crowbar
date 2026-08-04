//! `--surface spinner`: what taffy resolves a rotating glyph to, in a real
//! window.
//!
//! **The port turns**, so the two claims the capture rests on are measured here
//! rather than argued:
//!
//! * `the_first_frame_is_the_turns_origin` — the snapshot the driver emits is
//!   the *first* frame, and gpui stamps the animation's `start` on its first
//!   `request_layout`, so that frame is at `delta ≈ 0`. Asserted below 1e-3 of a
//!   turn, with a control that later frames really do advance.
//! * `the_recorded_box_never_moves_while_it_turns` — gpui's rotation is a
//!   paint-time transform, so the *layout* bounds the driver records are the
//!   same at every delta. The reference's are not: `WebKit`'s
//!   `getBoundingClientRect()` returns the transformed box and travels 6.63px.
//!   That asymmetry is why the **reference** must be pinned at
//!   `currentTime = 0` and the native side needs no pinning at all.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crate::driver_anchors::fold_text_halves;
use crate::row_surface::{Cell, RowSurface};
use crowbar_driver::{AnchorRegistry, Paint, RawAnchor};
use crowbar_ui::primitives::spinner::{self, Spinner};
use gpui::{Bounds, Pixels, TestAppContext, px};
use std::cell::RefCell;
use std::rc::Rc;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "spinner"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the glyph itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, spinner::ID_SPINNER, id)
}

/// The default cell is `loading-spinner.tsx`'s `size-4`: 16 × 16, the box the
/// reference reports, with no paint of any kind on it.
#[gpui::test]
fn the_default_cell_is_the_captured_sixteen_pixel_glyph(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec!["spinner".to_owned()],
        "the host row carries no anchor",
    );

    let root = at(&records, "spinner");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let record = find(&records, "spinner");
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert_eq!(
        record.background,
        Paint::None,
        "an <svg> paints no background"
    );
    assert!(
        record.text.is_none(),
        "the glyph has no text node, so the reference emits no fg and no font either",
    );
}

/// Every call site's box, and the one that moves at 640px.
///
/// The control is the other three: a port that read the breakpoint everywhere
/// would be wrong three times over, and this asserts each of them is unmoved.
#[gpui::test]
fn only_the_button_indicator_changes_across_the_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (site, expected) in [
        ("none", px(24.0)),
        ("loading-spinner", px(16.0)),
        ("loading-spinner-compact", px(12.0)),
    ] {
        for viewport in ["639", "1714"] {
            let records = measure(
                cx,
                cell(&["--call-site", site, "--viewport-width", viewport]),
            );
            let box_ = at(&records, "spinner");
            assert_px(box_.size.width, expected);
            assert_px(box_.size.height, expected);
        }
    }

    let narrow = measure(
        cx,
        cell(&[
            "--call-site",
            "button-loading-indicator",
            "--viewport-width",
            "639",
        ]),
    );
    let wide = measure(
        cx,
        cell(&[
            "--call-site",
            "button-loading-indicator",
            "--viewport-width",
            "1714",
        ]),
    );
    assert_px(at(&narrow, "spinner").size.width, px(18.0));
    assert_px(at(&wide, "spinner").size.width, px(16.0));
}

/// §8.3's `empty`: `size={0}` gives a **zero-area** box, which paints nothing —
/// `skeleton`'s cell, reached from the props. It overrides a call site that
/// would have pinned a box.
#[gpui::test]
fn the_empty_cell_has_no_area_and_overrides_every_call_site(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for site in ["none", "loading-spinner", "button-loading-indicator"] {
        let records = measure(cx, cell(&["--flags", "empty", "--call-site", site]));
        let root = at(&records, "spinner");
        assert_px(root.size.width, px(0.0));
        assert_px(root.size.height, px(0.0));
        assert!(
            !find(&records, "spinner").visible,
            "a zero-area box paints nothing: {site}",
        );
    }
}

/// **The first frame is the turn's origin**, which is what makes
/// `CROWBAR_ROW_SNAPSHOT` — "emit one snapshot of the first frame and quit" —
/// the native counterpart of pinning the reference at `currentTime = 0`.
///
/// Measured on the component's own [`Spinner::turn`] rather than on an animation
/// this test built: the instant depends on the duration, the repeat and the
/// easing together, and a locally-built animation would be measuring something
/// else.
///
/// The control is the second half: later frames really do advance, so this is
/// not passing because nothing animates.
#[gpui::test]
fn the_first_frame_is_the_turns_origin(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let deltas: Rc<RefCell<Vec<f32>>> = Rc::new(RefCell::new(Vec::new()));
    let window = {
        let deltas = deltas.clone();
        cx.open_window(gpui::size(px(64.0), px(64.0)), move |_, _| TurnProbe {
            deltas,
        })
    };
    cx.run_until_parked();

    let first = *deltas.borrow().first().expect("one frame was rendered");
    assert!(
        first < 1e-3,
        "the first frame must be the turn's origin, got {first} of a turn",
    );

    // The control: the animation is live, so a later frame is a different
    // instant. Without this the assertion above would pass on a static element.
    for _ in 0..3 {
        window
            .update(cx, |_, window, cx| window.simulate_next_frame(cx))
            .expect("the window is open");
        cx.run_until_parked();
    }
    let frames = deltas.borrow().len();
    assert!(frames > 1, "only {frames} frame(s) were rendered");
    assert!(
        deltas
            .borrow()
            .iter()
            .all(|delta| (0.0..=1.0).contains(delta)),
        "{:?}",
        deltas.borrow(),
    );
}

/// **The recorded box does not move while the glyph turns**, and the reference's
/// does — which is the asymmetry the whole capture rests on.
///
/// gpui rotates at paint time (`Window::paint_path` tessellates into the scene
/// and never reaches taffy), so the driver, which reads *layout* bounds at
/// prepaint, sees the same 16 × 16 at every delta. `WebKit`'s
/// `getBoundingClientRect()` returns the **transformed** box and travels 6.63px
/// over the same turn.
#[gpui::test]
fn the_recorded_box_never_moves_while_it_turns(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let cell = cell(&[]);
    let size = RowSurface::window_size(&cell);
    let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
    let window = cx.open_window(size, |_, _| super::Stage(cell));
    cx.run_until_parked();

    let at_rest = fold_text_halves(anchors.records());
    let first = relative_to(&at_rest, spinner::ID_SPINNER, spinner::ID_SPINNER);
    assert_px(first.size.width, px(16.0));
    assert_px(first.size.height, px(16.0));

    // Step well past the quarter turns, where a transformed box would be at its
    // widest — the 45° instant is the reference's 22.627.
    let mut frames = 0;
    for _ in 0..8 {
        frames += window
            .update(cx, |_, window, cx| window.simulate_next_frame(cx))
            .expect("the window is open");
        cx.run_until_parked();

        let records = fold_text_halves(anchors.records());
        let box_ = relative_to(&records, spinner::ID_SPINNER, spinner::ID_SPINNER);
        assert_px(box_.size.width, first.size.width);
        assert_px(box_.size.height, first.size.height);
        assert_px(box_.origin.x, first.origin.x);
        assert_px(box_.origin.y, first.origin.y);
    }
    // The control: those frames were real animation frames, so the assertion
    // above is about a turning glyph rather than a still one.
    assert!(frames > 0, "no animation frame was scheduled");
}

/// The probe [`the_first_frame_is_the_turns_origin`] drives: the component's own
/// animation, with the delta recorded instead of drawn.
struct TurnProbe {
    deltas: Rc<RefCell<Vec<f32>>>,
}

impl gpui::Render for TurnProbe {
    fn render(
        &mut self,
        _window: &mut gpui::Window,
        _cx: &mut gpui::Context<Self>,
    ) -> impl gpui::IntoElement {
        use gpui::AnimationExt as _;
        let deltas = self.deltas.clone();
        gpui::div().with_animation("turn-probe", Spinner::turn(), move |element, delta| {
            deltas.borrow_mut().push(delta);
            element
        })
    }
}
