//! What the row's layout actually resolves to, measured in a real window.
//!
//! Everything else about the row can be reasoned about from the source. This
//! cannot: the whole point of the Phase 1 gate is that **taffy and Blink might
//! disagree**, and the only honest way to know what taffy does with three
//! nested flex containers and a `max-w-[45%]` is to lay it out and read the
//! bounds back. So these assertions are written against the numbers gpui
//! produced, and the CSS arithmetic they are compared to is spelled out in each
//! one — where the two agree it is stated as an equality, and where they cannot
//! it is stated as a bound with the reason.
//!
//! Reading the bounds back is `crowbar-driver`'s extractor, taken as a
//! dev-dependency so this runs under a plain `cargo test --workspace`.
//!
//! # This file is the harness, and **nothing else**
//!
//! One file per surface, under `src/row_layout/`, and **no list** — the same
//! mechanism `src/surfaces/` uses and for the same reason. This file used to
//! carry all of it: 4083 lines, eight `mod` blocks, and every Tier B worker
//! appending a ninth at the end of it, which conflicted on four consecutive
//! merges because appending to one place from two worktrees is a conflict by
//! construction. `build.rs` now reads `src/row_layout/`, writes a `#[path]`
//! module declaration per `.rs` file, and the `include!` below pulls the result
//! in. Measuring a new surface is **adding a file**: not a `mod` line here, not
//! an import, nothing anyone else's branch also touched.
//!
//! What stays here is what all of them share — `Stage`, `measure`, `lay_out`,
//! and the recording and assertion helpers — because a second copy of `measure`
//! is a second thing to drift. A submodule reaches them by `use super::{…}`,
//! which is exactly what the inline `mod` blocks already did.
//!
//! [`lay_out`] is the reason that rule now has teeth. A wrapped
//! `gpui-component` popup is **not in the first draw**, so a surface that
//! carries one needs frames delivered by hand — and P3.15 delivered them in a
//! local copy of `measure_in` inside one surface's file, which is exactly the
//! second copy this paragraph warns about. It is here instead, on the signal
//! `crowbar_driver::Settling` carries, so a surface with a deferred popup is
//! again nothing but a file.
//!
//! The cost is `src/surfaces/`'s cost, stated there in full: the module list is
//! generated, so `git grep 'mod tabs'` finds nothing, and this comment is the
//! answer to that.

#![cfg(test)]

use crowbar_driver::{AnchorRegistry, Observation, RawAnchor, Settling};
use gpui::{
    Context, IntoElement, ParentElement as _, Pixels, Render, Size, Styled as _, TestAppContext,
    Window, div, px,
};

use crate::driver_anchors::{DriverAnchors, fold_text_halves};
use crate::row_surface::{Cell, RowSurface, render_row};

// The per-surface modules, declared by `build.rs` from `src/row_layout/`.
include!(concat!(env!("OUT_DIR"), "/row_layout_mods.rs"));

struct Stage(Cell);

impl Render for Stage {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let theme = self.0.theme();
        // Offset and width-pinned exactly as `RowSurface` draws it, and through
        // the same `render_row` dispatch, so the numbers here are the numbers
        // the oracle will read off the binary. The horizontal offset comes off
        // the cell for the same reason — a harness that always insetted by 24
        // would measure a full-bleed surface in a place the binary never draws
        // it.
        div()
            .pl(self.0.horizontal_inset_px())
            .pt(px(crate::row_surface::INSET_Y))
            .font_family(theme.font_sans.primary().unwrap_or("sans-serif"))
            .child(
                div()
                    .w(self.0.width_px())
                    .child(render_row(&self.0, &theme, &DriverAnchors)),
            )
    }
}

/// Lays the cell out in a real window and hands back what the extractor
/// recorded — folded, so what these assertions see is what a snapshot would
/// carry.
///
/// The window is `RowSurface::window_size`, exactly as the app makes it — on
/// **both** axes since P2.5. Sizing it to some fixed 1200px instead would let a
/// surface wider than the window pass unnoticed here and clip in the real
/// binary, and it would make the harness and the thing it measures two
/// different configurations; the height used to be a fixed 400 for no better
/// reason than that no surface had yet been tall enough for it to matter.
fn measure(cx: &mut TestAppContext, cell: Cell) -> Vec<RawAnchor> {
    let window = RowSurface::window_size(&cell);
    measure_in(cx, cell, window).1
}

/// [`measure`] in a window of a stated size, with the registry kept.
///
/// Two callers need one of the two extras. `row_layout::window` drives a window
/// the surface does **not** fit in, which is the case a harness that always
/// sized itself correctly could never reach; and `row_snapshot::emit` takes the
/// registry, so a test of the refusal has to hold it rather than only its
/// records.
fn measure_in(
    cx: &mut TestAppContext,
    cell: Cell,
    window: Size<Pixels>,
) -> (AnchorRegistry, Vec<RawAnchor>) {
    lay_out(cx, window, |_, _| Stage(cell))
}

/// Lays **any** view out and hands back the frame it settled on.
///
/// # Why this is not one `open_window` and one `run_until_parked`
///
/// It was, and that was one draw — which every surface up to P3.15 was, because
/// every one of them was a pure function of its props. A deferred anchored popup
/// is not. `gpui_component::Popover` renders its popup only once the trigger's
/// `prepaint` has measured it, and `prepaint` runs after `render` has returned,
/// so the first draw is the trigger and nothing else; `select`'s popup *is* in
/// the first draw but sized off a still-default `Bounds`, which is worse,
/// because 2px wide is a number and it reads as a port defect.
///
/// gpui's own comment says why the vendor's escape hatch is not one here:
/// `Window::request_animation_frame` is `on_next_frame`, and *"tests have no
/// platform frame loop"*, so `run_until_parked` — which drives the executor, not
/// the frame loop — never delivers the frame the vendor asked for. The frames
/// are therefore delivered by hand, and the loop stops on the signal
/// [`crowbar_driver::Settling`] carries: **the first draw that reproduced the
/// previous draw's anchors**. `crowbar-driver`'s `src/frame.rs` documents it in
/// full; the binary's capture stops on the same one, which is what keeps these
/// assertions and the emitted snapshot statements about the same frame.
///
/// A surface that needs one draw pays for one extra look and no extra draw: gpui
/// redraws only a dirty window, so an unchanged picture settles immediately.
fn lay_out<V, B>(
    cx: &mut TestAppContext,
    window: Size<Pixels>,
    build: B,
) -> (AnchorRegistry, Vec<RawAnchor>)
where
    V: Render + 'static,
    B: FnOnce(&mut Window, &mut Context<V>) -> V,
{
    // `main.rs` calls this before it opens its window, so the harness calls it
    // before it opens its own — a wrapped `gpui-component` widget reaches
    // `GlobalState` on the way to opening, and a harness that skipped the
    // install would measure a configuration the binary never runs. It is inert
    // for a surface that wraps nothing: every assertion in this tree was written
    // before it was here and none of them moved.
    cx.update(gpui_component::init);
    let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
    let handle = cx.open_window(window, build);
    cx.run_until_parked();

    let mut settling = Settling::watching(&anchors);
    loop {
        match settling.observe() {
            Observation::Settled => break,
            Observation::Changing => {}
            Observation::NeverSettled => panic!(
                "the recorded frame changed on every draw, so this surface has no settled \
                 picture: {:?}",
                ids(&anchors.records()),
            ),
        }
        handle
            .update(cx, |_, window, cx| window.simulate_next_frame(cx))
            .expect("the window is open");
        cx.run_until_parked();
    }

    let records = fold_text_halves(anchors.records());
    (anchors, records)
}

fn find(records: &[RawAnchor], id: &str) -> RawAnchor {
    records
        .iter()
        .find(|record| record.id == id)
        .unwrap_or_else(|| panic!("{id} was not recorded; got {:?}", ids(records)))
        .clone()
}

fn ids(records: &[RawAnchor]) -> Vec<String> {
    records.iter().map(|r| r.id.to_string()).collect()
}

/// Bounds relative to a named root anchor, which is the space the contract
/// compares in (`native/oracle/ANCHORS.md` §4).
fn relative_to(records: &[RawAnchor], root: &str, id: &str) -> gpui::Bounds<Pixels> {
    let origin = find(records, root).bounds.origin;
    let mut bounds = find(records, id).bounds;
    bounds.origin -= origin;
    bounds
}

#[track_caller]
fn assert_px(actual: Pixels, expected: Pixels) {
    assert!(
        (actual - expected).abs() < px(0.01),
        "expected {expected:?}, got {actual:?}",
    );
}

/// `native/oracle/ANCHORS.md` §5's bounds tolerance.
///
/// Used where gpui's layout is **rounded to whole pixels** and the CSS value is
/// not — see `a_percentage_length_lands_on_a_whole_pixel`. Everywhere else the
/// assertion above is exact, so this spelling marks the places where the two
/// engines genuinely cannot agree to the last decimal.
#[track_caller]
fn assert_within_tolerance(actual: Pixels, expected: Pixels) {
    assert!(
        (actual - expected).abs() <= px(0.5),
        "expected {expected:?} ± 0.5, got {actual:?}",
    );
}

fn a_cell(args: &[&str]) -> Cell {
    Cell::parse(args.iter().map(|arg| (*arg).to_owned())).expect("a well-formed cell")
}

/// The harness's own regression test.
///
/// Every other test in this tree measures a surface. These two measure
/// [`lay_out`], because the thing that broke was not any surface's layout — it
/// was **which frame** the harness read, and no assertion about a static
/// surface's boxes can see that. A wrapped `gpui-component` popup is the case
/// where it is visible, and the shared harness has to hold it or every such
/// surface hand-rolls its own frame delivery and they drift.
mod deferred {
    use std::cell::Cell as MutableCell;
    use std::rc::Rc;

    use crowbar_driver::{anchor, anchor_root};
    use gpui::{
        Context, IntoElement, ParentElement as _, Render, Styled as _, TestAppContext, Window,
        canvas, div, px, size,
    };

    use super::{ids, lay_out};

    /// The anchors this stage draws, once it can.
    const POPUP: &str = "deferred-popup";
    const VIEWPORT: &str = "deferred-viewport";

    /// A stage whose root anchor **does not exist on the first draw**.
    ///
    /// This is `gpui_component::Popover`'s mechanism, reproduced rather than
    /// imported so that the harness's proof does not move when the vendor's
    /// popup does. The vendor gates its popup behind
    /// `if !open || !trigger_bounds_captured` and sets the second half from the
    /// trigger's `on_prepaint` — which `gpui-component` implements as an
    /// absolutely-positioned `canvas` child, so the callback runs during
    /// `prepaint`, after `render` has already returned. On the first capture it
    /// calls `window.request_animation_frame()`, and only the frame that
    /// delivers has the popup in it.
    struct Deferred {
        measured: Rc<MutableCell<bool>>,
    }

    impl Render for Deferred {
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
                POPUP,
                div()
                    .w(px(256.0))
                    .h(px(177.0))
                    .child(trigger)
                    .child(anchor(VIEWPORT, div().w(px(254.0)).h(px(175.0)))),
            )
            .into_any_element()
        }
    }

    /// **The shared harness lays out a deferred popup.**
    ///
    /// This is the assertion that fails on the one-draw harness, and it fails
    /// the way the binary failed: not with wrong numbers but with nothing at all
    /// — `[]`, because the root anchor is behind a flag that `prepaint` sets
    /// after `render` has returned.
    #[gpui::test]
    fn the_shared_harness_reaches_a_popup_that_needs_a_second_draw(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let (_anchors, records) = lay_out(cx, size(px(400.0), px(300.0)), |_, _| Deferred {
            measured: Rc::new(MutableCell::new(false)),
        });

        assert_eq!(ids(&records), [POPUP, VIEWPORT]);
    }

    /// And the boxes it reaches are the **laid-out** ones, not a default
    /// `Bounds`.
    ///
    /// `select` is the reason this is asserted separately: its popup *is* in the
    /// first draw, sized off a `Bounds::default()` its trigger has not filled in
    /// yet, and comes out 2px wide. A harness that only checked the anchor was
    /// present would call that a pass and hand the number to the differ, where
    /// it reads as a port defect rather than as a frame that was captured too
    /// early.
    #[gpui::test]
    fn the_deferred_popup_carries_its_real_geometry(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let (_anchors, records) = lay_out(cx, size(px(400.0), px(300.0)), |_, _| Deferred {
            measured: Rc::new(MutableCell::new(false)),
        });

        let popup = super::find(&records, POPUP);
        super::assert_px(popup.bounds.size.width, px(256.0));
        super::assert_px(popup.bounds.size.height, px(177.0));
        super::assert_px(
            super::relative_to(&records, POPUP, VIEWPORT).size.width,
            px(254.0),
        );
    }
}
