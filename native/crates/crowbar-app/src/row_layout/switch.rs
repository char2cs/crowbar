//! `--surface switch`: what taffy resolves a toggle to, in a real window.
//!
//! **These assertions are written against the live reference**, which is what
//! makes them different from `skeleton`'s: `/tmp/p3-ref-switch.json` and
//! `/tmp/p3-ref-switch-selected.json` were captured from two switches that were
//! simultaneously on screen in the app's Git settings tab, and the numbers below
//! are theirs. Where the reference reports a value it is asserted as an equality
//! and named as the reference's.
//!
//! The thumb's slide is the one place the two engines express the same picture
//! differently — `translate` there, layout here (see
//! `crowbar_ui::components::switch`) — so the test that matters most is that the
//! **recorded box** lands where `WebKit`'s does, on both cells.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::switch;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "switch"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the track.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, switch::ID_SWITCH, id)
}

/// The solid colour an anchor paints, or a panic naming what it painted instead.
#[track_caller]
fn solid(records: &[RawAnchor], id: &str) -> gpui::Hsla {
    match find(records, id).background {
        Paint::Solid(colour) => colour,
        other => panic!("{id} painted {other:?} rather than a solid colour"),
    }
}

/// **The resting cell is the live reference**, anchor for anchor:
/// `switch` at `(0, 0, 30, 18)` and `switch-thumb` at `(1, 1, 16, 16)`.
#[gpui::test]
fn the_resting_cell_is_the_live_reference(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec!["switch".to_owned(), "switch-thumb".to_owned()],
        "exactly the surface's own two anchors, in document order",
    );

    let track = at(&records, "switch");
    assert_px(track.origin.x, px(0.0));
    assert_px(track.origin.y, px(0.0));
    assert_px(track.size.width, px(30.0));
    assert_px(track.size.height, px(18.0));

    // `p-px` puts the thumb one pixel in on both axes — the reference's (1, 1).
    let thumb = at(&records, "switch-thumb");
    assert_px(thumb.origin.x, px(1.0));
    assert_px(thumb.origin.y, px(1.0));
    assert_px(thumb.size.width, px(16.0));
    assert_px(thumb.size.height, px(16.0));
}

/// **`selected` moves the thumb's recorded box from `x: 1` to `x: 13`** — the
/// live reference's two numbers, and the reason this surface exists.
///
/// The original expresses the move as `translate: 12px`, which `WebKit` folds into
/// `getBoundingClientRect()`; the port expresses it as layout, which the driver
/// reads at prepaint. This asserts the two land in the same place, which is the
/// whole claim of that design decision.
#[gpui::test]
fn selected_slides_the_thumb_to_the_references_x(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let off = measure(cx, cell(&[]));
    let on = measure(cx, cell(&["--flags", "selected"]));

    assert_px(at(&off, "switch-thumb").origin.x, px(1.0));
    assert_px(at(&on, "switch-thumb").origin.x, px(13.0));

    // Only `x` moves. `y`, both extents and the track are identical, exactly as
    // the two reference files are.
    assert_px(at(&on, "switch-thumb").origin.y, px(1.0));
    assert_px(at(&on, "switch-thumb").size.width, px(16.0));
    assert_px(at(&on, "switch-thumb").size.height, px(16.0));
    assert_px(at(&on, "switch").size.width, at(&off, "switch").size.width);
    assert_px(
        at(&on, "switch").size.height,
        at(&off, "switch").size.height,
    );

    // And the thumb stays inside the track: 13 + 16 = 29 = 30 − 1 padding pixel.
    let track = at(&on, "switch");
    let thumb = at(&on, "switch-thumb");
    assert_px(
        thumb.origin.x + thumb.size.width,
        track.size.width - px(1.0),
    );
}

/// **`selected` moves the track's background and leaves the thumb's alone** —
/// the second of the two fields, on the other anchor.
#[gpui::test]
fn selected_repaints_the_track_and_not_the_thumb(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let off = measure(cx, cell(&["--theme", "dark"]));
    let on = measure(cx, cell(&["--flags", "selected", "--theme", "dark"]));

    // The track: `bg-input` → `bg-primary`.
    assert_eq!(solid(&off, "switch"), Theme::DARK.input.value());
    assert_eq!(solid(&on, "switch"), Theme::DARK.primary.value());
    assert_ne!(solid(&off, "switch"), solid(&on, "switch"));

    // The thumb: `bg-background` on both, which the reference reports as
    // `#1f1f1eff` twice. A port that tinted the checked thumb would look like an
    // improvement and be wrong.
    assert_eq!(solid(&off, "switch-thumb"), Theme::DARK.background.value());
    assert_eq!(solid(&on, "switch-thumb"), Theme::DARK.background.value());
    assert_eq!(solid(&off, "switch-thumb"), solid(&on, "switch-thumb"));
}

/// **`rounded-full` is `f32::MAX` and the thumb's corner is 16** — the two radii
/// the reference reports, on one surface.
#[gpui::test]
fn the_track_and_the_thumb_take_different_corners(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(find(&records, "switch").radius, px(f32::MAX));
    assert_px(find(&records, "switch-thumb").radius, px(16.0));

    // Both anchors paint **no** border — `kbd`'s side of the trap, compared
    // exactly by the differ.
    assert_px(find(&records, "switch").border_width, px(0.0));
    assert_px(find(&records, "switch-thumb").border_width, px(0.0));

    // And both are painted, which is the column that caught three bad captures.
    assert!(find(&records, "switch").visible);
    assert!(find(&records, "switch-thumb").visible);
}

/// Neither anchor paints text, so the whole text group is absent on this side —
/// which is what makes `--content` vacuous rather than merely unexercised.
#[gpui::test]
fn no_anchor_on_this_surface_paints_text(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec![],
        vec!["--content", "overflow"],
        vec!["--flags", "selected"],
    ] {
        let records = measure(cx, cell(&args));
        for id in ["switch", "switch-thumb"] {
            assert!(
                find(&records, id).text.is_none(),
                "{id} painted text on {args:?}",
            );
        }
    }
}

/// **`--width` moves nothing**, because both track axes are authored — the
/// control that makes the surface's "vacuous" caption a measurement.
#[gpui::test]
fn the_width_axis_moves_nothing_on_this_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(cx, cell(&["--width", "200"]));
    let wide = measure(cx, cell(&["--width", "600"]));

    for id in ["switch", "switch-thumb"] {
        assert_px(at(&narrow, id).size.width, at(&wide, id).size.width);
        assert_px(at(&narrow, id).size.height, at(&wide, id).size.height);
        assert_px(at(&narrow, id).origin.x, at(&wide, id).origin.x);
    }
}

/// The `sm:` arm is the live one and the base arm is a genuinely bigger switch —
/// laid out rather than computed, so taffy is what answers.
#[gpui::test]
fn the_base_breakpoint_lays_out_a_bigger_switch(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let base = measure(
        cx,
        cell(&["--viewport-width", "500", "--flags", "selected"]),
    );

    assert_px(at(&base, "switch").size.width, px(38.0));
    assert_px(at(&base, "switch").size.height, px(22.0));
    assert_px(at(&base, "switch-thumb").size.width, px(20.0));
    // `t − 4` at t = 20 is 16, plus the padding pixel.
    assert_px(at(&base, "switch-thumb").origin.x, px(17.0));
    // Still flush: 17 + 20 = 37 = 38 − 1.
    assert_px(
        at(&base, "switch-thumb").origin.x + at(&base, "switch-thumb").size.width,
        at(&base, "switch").size.width - px(1.0),
    );
}
