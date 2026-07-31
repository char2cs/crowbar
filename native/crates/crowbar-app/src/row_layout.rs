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
//! What stays here is what all of them share — `Stage`, `measure`, and the
//! recording and assertion helpers — because a second copy of `measure` is a
//! second thing to drift. A submodule reaches them by `use super::{…}`, which
//! is exactly what the inline `mod` blocks already did.
//!
//! The cost is `src/surfaces/`'s cost, stated there in full: the module list is
//! generated, so `git grep 'mod tabs'` finds nothing, and this comment is the
//! answer to that.

#![cfg(test)]

use crowbar_driver::{AnchorRegistry, RawAnchor};
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
    let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
    let _window = cx.open_window(window, |_, _| Stage(cell));
    cx.run_until_parked();
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
