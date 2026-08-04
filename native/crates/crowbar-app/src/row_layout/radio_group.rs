//! `--surface radio-group`, laid out in a real window.
//!
//! **There is no live reference for this surface** — `radio-group.tsx`'s only
//! importer needs a child branch with an unprotected local parent, and this
//! item's dev environment has none. See `crowbar_ui::primitives::radio_group`
//! and `native/mapping/radio-group.md`. What this file establishes instead is
//! the arithmetic against `radio-group.tsx`'s own compiled classes, measured
//! by injecting them into the live app's DOM — the same values
//! `crowbar_ui::primitives::radio_group`'s constants carry, laid out here in
//! a real window rather than only unit-tested against the theme.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::radio_group;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "radio-group"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to this surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, radio_group::ID_GROUP, id)
}

/// **The resting cell**: a 16px circle at `sm:`, one pixel of border, radius
/// `f32::MAX`.
#[gpui::test]
fn the_resting_cell_is_a_sixteen_pixel_circle(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let circle = at(&records, radio_group::ID_RADIO);

    assert_px(circle.size.width, px(16.0));
    assert_px(circle.size.height, px(16.0));

    let record = find(&records, radio_group::ID_RADIO);
    assert_px(record.radius, radio_group::RADIUS);
    assert_px(record.border_width, radio_group::BORDER_WIDTH);
    assert!(record.visible);
    assert!(record.text.is_none(), "a radio paints no text");
}

/// **The anchor set is two, and does not change with the state** — the fill
/// and its dot stay unanchored on every cell, `checkbox.rs`'s own guard
/// reproduced on this control.
#[gpui::test]
fn the_anchor_set_is_two_and_does_not_change_with_the_state(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec![],
        vec!["--flags", "selected"],
        vec!["--disabled"],
        vec!["--invalid"],
        vec!["--theme", "light"],
    ] {
        let records = measure(cx, cell(&args));
        let mut seen = ids(&records);
        seen.sort();
        let mut expected = vec![
            radio_group::ID_GROUP.to_owned(),
            radio_group::ID_RADIO.to_owned(),
        ];
        expected.sort();
        assert_eq!(
            seen, expected,
            "the fill and its dot must stay unanchored: {args:?}",
        );
    }
}

/// **`selected` repaints the circle in dark and not in light** — laid out
/// both ways, so the vacuous half is a measurement rather than a claim.
#[gpui::test]
fn selected_repaints_the_circle_in_dark_and_not_in_light(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark_off = measure(cx, cell(&["--theme", "dark"]));
    let dark_on = measure(cx, cell(&["--flags", "selected", "--theme", "dark"]));
    assert_ne!(
        find(&dark_off, radio_group::ID_RADIO).background,
        find(&dark_on, radio_group::ID_RADIO).background,
    );
    assert_eq!(
        find(&dark_on, radio_group::ID_RADIO).background,
        Paint::Solid(Theme::DARK.background.value()),
    );

    let light_off = measure(cx, cell(&["--theme", "light"]));
    let light_on = measure(cx, cell(&["--flags", "selected", "--theme", "light"]));
    assert_eq!(
        find(&light_off, radio_group::ID_RADIO).background,
        find(&light_on, radio_group::ID_RADIO).background,
        "the light table measures the same colour twice",
    );
}

/// **`aria-invalid` moves the border colour and nothing else** — driven by
/// `--invalid` because every surface must declare `error` unmodelled.
#[gpui::test]
fn invalid_moves_the_border_colour_only(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&["--theme", "dark"]));
    let invalid = measure(cx, cell(&["--invalid", "--theme", "dark"]));

    assert_ne!(
        find(&invalid, radio_group::ID_RADIO).border_color,
        find(&resting, radio_group::ID_RADIO).border_color,
    );
    assert_px(
        find(&invalid, radio_group::ID_RADIO).border_width,
        find(&resting, radio_group::ID_RADIO).border_width,
    );
    assert_px(
        at(&invalid, radio_group::ID_RADIO).size.width,
        at(&resting, radio_group::ID_RADIO).size.width,
    );
}

/// **`--width` moves nothing**, because `size-*` authors both axes.
#[gpui::test]
fn the_width_axis_moves_nothing_on_this_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(cx, cell(&["--width", "200"]));
    let wide = measure(cx, cell(&["--width", "600"]));

    assert_px(
        at(&narrow, radio_group::ID_RADIO).size.width,
        at(&wide, radio_group::ID_RADIO).size.width,
    );
}

/// The `sm:` arm is the smaller circle; the base arm is 18px.
#[gpui::test]
fn the_base_breakpoint_lays_out_a_bigger_circle(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let base = measure(cx, cell(&["--viewport-width", "500"]));
    assert_px(at(&base, radio_group::ID_RADIO).size.width, px(18.0));
    assert_px(at(&base, radio_group::ID_RADIO).size.height, px(18.0));
    // `rounded-full` does not become a different number at the base
    // breakpoint — it is `f32::MAX` on both.
    assert_px(
        find(&base, radio_group::ID_RADIO).radius,
        radio_group::RADIUS,
    );
}

/// `gap-3` on the group — the one thing the group's own root box authors,
/// confirmed as a real length rather than merely present.
#[gpui::test]
fn the_group_gap_is_the_compiled_spacing_multiple(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    // The single anchored radio sits at the group's own padding-free origin —
    // `gap-3` only separates *siblings*, and this surface anchors one.
    let circle = at(&records, radio_group::ID_RADIO);
    assert_px(circle.origin.x, px(0.0));
    assert_px(circle.origin.y, px(0.0));
    assert_eq!(radio_group::GROUP_GAP, px(12.0));
}
