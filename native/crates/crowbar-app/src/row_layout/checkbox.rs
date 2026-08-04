//! `--surface checkbox`: what taffy resolves a checkbox to, in a real window.
//!
//! Written against the live reference — `/tmp/p3-ref-checkbox.json` and
//! `/tmp/p3-ref-checkbox-selected.json`, captured from the commit popover's file
//! list.
//!
//! The load-bearing assertion here is not a geometry one. It is
//! [`the_anchor_set_is_one_and_does_not_change_with_the_state`]: the surface has
//! **one** anchor in every cell, which is what keeps the resting cell comparable
//! against a reference whose `display: none` indicator the DOM extractor emits
//! and this one does not. See `crowbar_ui::primitives::checkbox`'s module docs.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::checkbox;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "checkbox"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the box itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, checkbox::ID_CHECKBOX, id)
}

#[track_caller]
fn solid(records: &[RawAnchor], id: &str) -> gpui::Hsla {
    match find(records, id).background {
        Paint::Solid(colour) => colour,
        other => panic!("{id} painted {other:?} rather than a solid colour"),
    }
}

/// **The resting cell is the live reference**: `16 × 16` at the origin, radius 4,
/// a 1px border, painted.
#[gpui::test]
fn the_resting_cell_is_the_live_reference(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let box_ = at(&records, "checkbox");
    assert_px(box_.origin.x, px(0.0));
    assert_px(box_.origin.y, px(0.0));
    assert_px(box_.size.width, px(16.0));
    assert_px(box_.size.height, px(16.0));

    let record = find(&records, "checkbox");
    assert_px(record.radius, px(4.0));
    // `button`'s trap: a real 1px border, compared exactly.
    assert_px(record.border_width, px(1.0));
    assert!(record.visible);
    assert!(record.text.is_none(), "a checkbox paints no text");
}

/// **The anchor set is one, in every cell** — which is the decision the whole
/// component turns on.
///
/// The indicator is `display: none` when unchecked, and the two extractors
/// disagree about what that is: `extract.ts` emits a zero-box anchor and this
/// side has none at all. Rather than let the sets differ *by state*, the fill
/// carries no anchor on either side. This test is what stops a later change
/// re-introducing the problem quietly.
#[gpui::test]
fn the_anchor_set_is_one_and_does_not_change_with_the_state(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec![],
        vec!["--flags", "selected"],
        vec!["--indeterminate"],
        vec!["--disabled"],
        vec!["--invalid"],
        vec!["--theme", "light"],
    ] {
        let records = measure(cx, cell(&args));
        assert_eq!(
            ids(&records),
            vec!["checkbox".to_owned()],
            "the fill and the tick must stay unanchored: {args:?}",
        );
    }
}

/// **`selected` moves the background in dark and nothing at all in light** —
/// laid out both ways, so the vacuous half is a measurement rather than a claim.
#[gpui::test]
fn selected_repaints_the_box_in_dark_and_not_in_light(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark_off = measure(cx, cell(&["--theme", "dark"]));
    let dark_on = measure(cx, cell(&["--flags", "selected", "--theme", "dark"]));
    assert_ne!(solid(&dark_off, "checkbox"), solid(&dark_on, "checkbox"));
    assert_eq!(solid(&dark_on, "checkbox"), Theme::DARK.background.value());

    // The light table: the same colour twice. This cell cannot fail, and the
    // assertion says so rather than leaving it to a green run.
    let light_off = measure(cx, cell(&["--theme", "light"]));
    let light_on = measure(cx, cell(&["--flags", "selected", "--theme", "light"]));
    assert_eq!(solid(&light_off, "checkbox"), solid(&light_on, "checkbox"));
    assert_eq!(
        solid(&light_on, "checkbox"),
        Theme::LIGHT.background.value(),
    );
}

/// **Nothing else moves with `selected`, in either theme** — the geometry and the
/// border the two reference files report identically.
#[gpui::test]
fn selected_moves_no_geometry_and_no_border(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for theme in ["dark", "light"] {
        let off = measure(cx, cell(&["--theme", theme]));
        let on = measure(cx, cell(&["--flags", "selected", "--theme", theme]));

        assert_px(
            at(&on, "checkbox").size.width,
            at(&off, "checkbox").size.width,
        );
        assert_px(
            at(&on, "checkbox").size.height,
            at(&off, "checkbox").size.height,
        );
        assert_px(find(&on, "checkbox").radius, find(&off, "checkbox").radius);
        assert_px(
            find(&on, "checkbox").border_width,
            find(&off, "checkbox").border_width,
        );
        assert_eq!(
            find(&on, "checkbox").border_color,
            find(&off, "checkbox").border_color,
            "{theme}: only the background moves",
        );
    }
}

/// **`aria-invalid` moves the border colour and nothing else** — a compared
/// field, driven by `--invalid` because every surface must declare `error`
/// unmodelled.
#[gpui::test]
fn invalid_moves_the_border_colour_only(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&["--theme", "dark"]));
    let invalid = measure(cx, cell(&["--invalid", "--theme", "dark"]));

    assert_ne!(
        find(&invalid, "checkbox").border_color,
        find(&resting, "checkbox").border_color,
    );
    assert_px(
        find(&invalid, "checkbox").border_width,
        find(&resting, "checkbox").border_width,
    );
    assert_px(
        at(&invalid, "checkbox").size.width,
        at(&resting, "checkbox").size.width,
    );
}

/// **`--width` moves nothing**, because `size-*` authors both axes.
#[gpui::test]
fn the_width_axis_moves_nothing_on_this_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(cx, cell(&["--width", "200"]));
    let wide = measure(cx, cell(&["--width", "600"]));

    assert_px(
        at(&narrow, "checkbox").size.width,
        at(&wide, "checkbox").size.width,
    );
    assert_px(
        at(&narrow, "checkbox").size.height,
        at(&wide, "checkbox").size.height,
    );
}

/// The `sm:` arm is the live one; the base arm is an 18px box.
#[gpui::test]
fn the_base_breakpoint_lays_out_a_bigger_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let base = measure(cx, cell(&["--viewport-width", "500"]));
    assert_px(at(&base, "checkbox").size.width, px(18.0));
    assert_px(at(&base, "checkbox").size.height, px(18.0));
    // The corner is an arbitrary value and does **not** follow the breakpoint.
    assert_px(find(&base, "checkbox").radius, px(4.0));
}
