//! `--surface crowbar-mark`: what taffy resolves the brand mark to, in a real
//! window, against the captured reference at `/tmp/p3-ref-crowbar-mark.json`.
//!
//! The reference's numbers are `18 × 18`, `bg #00000000`, `radius 0`,
//! `border.w 0`, `visible: true`. **Five fields is the whole of it** — an
//! `<svg>`'s paint has no representation in `native/oracle/ANCHORS.md` — so what
//! these assertions can cover is the box, and they say so rather than implying
//! more.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::crowbar_mark;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "crowbar-mark"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the mark itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, crowbar_mark::ID_CROWBAR_MARK, id)
}

/// **The default cell is the captured tab-bar icon** — every field of it this
/// harness can see, which is every field the contract has for it.
#[gpui::test]
fn the_default_cell_is_the_captured_tab_bar_icon(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["crowbar-mark".to_owned()]);

    let root = at(&records, "crowbar-mark");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(18.0));
    assert_px(root.size.height, px(18.0));

    let record = find(&records, "crowbar-mark");
    assert!(record.visible);
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert_eq!(
        record.background,
        Paint::None,
        "the mark paints no background of its own",
    );
    assert!(
        record.text.is_none(),
        "an <svg> has no text nodes, so the anchor carries no run — this is the \
         assertion that would catch a port drawing the wordmark's lettering as a string",
    );
    assert!(!record.content_sized);
    assert!(!record.line_sized);
}

/// **The mark overflows its slot and is not shrunk to it.** The call site's own
/// comment says the 18px glyph in a 14px `place-content-center` box is
/// deliberate; this is the measurement that a `shrink-0` box inside a smaller
/// parent keeps its extent under taffy.
///
/// The slot carries no anchor, so the anchor set is unchanged — which is also
/// asserted, because an id appearing on a wrapper would be an anchor the React
/// side has no way to place.
#[gpui::test]
fn the_mark_keeps_its_extent_inside_the_smaller_slot(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--in-slot"]));

    assert_eq!(
        ids(&records),
        vec!["crowbar-mark".to_owned()],
        "the slot is not an anchor",
    );

    let root = at(&records, "crowbar-mark");
    assert_px(root.size.width, px(18.0));
    assert_px(root.size.height, px(18.0));
    assert!(
        f32::from(root.size.width) > f32::from(crowbar_mark::TAB_BAR_SLOT),
        "the mark should overrun the 14px slot: {:?}",
        root.size,
    );
    assert!(find(&records, "crowbar-mark").visible);
}

/// §8.3's `empty`: the extent merged away. The box has **no area**, and
/// `visible` is the one field that says so — the same column that caught two
/// worthless captures on `input` and on the carousel.
#[gpui::test]
fn the_empty_cell_has_no_area_and_reports_invisible(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec!["--flags", "empty"],
        vec!["--flags", "empty", "--in-slot"],
    ] {
        let records = measure(cx, cell(&args));
        let root = at(&records, "crowbar-mark");
        assert_px(root.size.width, px(0.0));
        assert_px(root.size.height, px(0.0));
        assert!(!find(&records, "crowbar-mark").visible, "{args:?}");
    }
}

/// **Every recorded field is theme-invariant**, which is this surface's
/// headline finding rather than an omission: the mark's ink reaches the contract
/// through no field at all, so the light and dark cells are the same picture.
///
/// A control as much as an assertion — if a port ever gave the mark a
/// background or a corner, this is where the two appearances would part.
#[gpui::test]
fn the_two_appearances_record_the_same_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark = measure(cx, cell(&["--theme", "dark"]));
    let light = measure(cx, cell(&["--theme", "light"]));

    let one = find(&dark, "crowbar-mark");
    let other = find(&light, "crowbar-mark");
    assert_px(one.bounds.size.width, other.bounds.size.width);
    assert_px(one.bounds.size.height, other.bounds.size.height);
    assert_eq!(one.background, other.background);
    assert_px(one.radius, other.radius);
    assert_px(one.border_width, other.border_width);
    assert_eq!(one.visible, other.visible);
}

/// The `sm` breakpoint moves nothing here: neither the primitive nor its call
/// site carries a `sm:` variant. Written down because on the *other* three P3.8
/// surfaces the same option is real, and "vacuous" is a measurement.
#[gpui::test]
fn the_breakpoint_moves_nothing(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "1714"]));
    let narrow = measure(cx, cell(&["--viewport-width", "500"]));
    assert_px(
        at(&wide, "crowbar-mark").size.width,
        at(&narrow, "crowbar-mark").size.width,
    );
    assert_px(
        at(&wide, "crowbar-mark").size.height,
        at(&narrow, "crowbar-mark").size.height,
    );
}
