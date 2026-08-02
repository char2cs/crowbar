//! `--surface slider`: what taffy resolves the track, the fill and the thumb
//! to, in a real window.
//!
//! **These assertions are written against the live reference**, captured in
//! the Tauri webview (not a Chrome surrogate — `border-radius: f32::MAX` is a
//! `WebKit` resolution): `/tmp/p3-ref-slider.json` (`value: 0`) and
//! `/tmp/p3-ref-slider-selected.json` (`value: 40`), both from the same live
//! "Workspaces" fault-injection row, `668px` wide at a 1714px window. Where
//! the reference reports a value it is asserted as an equality and named as
//! the reference's.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::slider;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "slider", "--width", "668"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the (invisible)
/// control.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, slider::ID_ROOT, id)
}

/// The solid colour an anchor paints, or a panic naming what it painted
/// instead.
#[track_caller]
fn solid(records: &[RawAnchor], id: &str) -> gpui::Hsla {
    match find(records, id).background {
        Paint::Solid(colour) => colour,
        other => panic!("{id} painted {other:?} rather than a solid colour"),
    }
}

/// **The resting cell is the live reference**, anchor for anchor:
/// `slider` at `(0, 0, 668, 4)`, `slider-track` at `(2, 0, 664, 4)`,
/// `slider-indicator` at `(2, 0, 8, 4)`, `slider-thumb` at `(0, -6, 16, 16)`.
#[gpui::test]
fn the_resting_cell_is_the_live_reference(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec![
            "slider".to_owned(),
            "slider-track".to_owned(),
            "slider-indicator".to_owned(),
            "slider-thumb".to_owned(),
        ],
        "exactly the surface's own four anchors, in document order",
    );

    let root = at(&records, "slider");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(668.0));
    assert_px(root.size.height, px(4.0));

    let track = at(&records, "slider-track");
    assert_px(track.origin.x, px(2.0));
    assert_px(track.origin.y, px(0.0));
    assert_px(track.size.width, px(664.0));
    assert_px(track.size.height, px(4.0));

    let indicator = at(&records, "slider-indicator");
    assert_px(indicator.origin.x, px(2.0));
    assert_px(indicator.origin.y, px(0.0));
    assert_px(indicator.size.width, px(8.0));
    assert_px(indicator.size.height, px(4.0));

    let thumb = at(&records, "slider-thumb");
    assert_px(thumb.origin.x, px(0.0));
    assert_px(thumb.origin.y, px(-6.0));
    assert_px(thumb.size.width, px(16.0));
    assert_px(thumb.size.height, px(16.0));
}

/// **`--value 40` lands on the second live reference's own numbers** —
/// `slider-indicator` at `w: 268.8` and `slider-thumb` at `x: 260.8`, both
/// read off `/tmp/p3-ref-slider-selected.json`. Both are asserted **within
/// tolerance**, not exactly: `268.8` and `260.8` are not whole pixels, and
/// gpui's own layout — unlike the arithmetic in
/// `crowbar_ui::components::slider` — rounds to one (measured: `269`), the
/// same reason `a_percentage_length_lands_on_a_whole_pixel` exists.
#[gpui::test]
fn value_forty_lands_on_the_second_live_references_numbers(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--value", "40"]));

    let indicator = at(&records, "slider-indicator");
    assert_px(indicator.origin.x, px(2.0));
    assert_within_tolerance(indicator.size.width, px(268.8));

    let thumb = at(&records, "slider-thumb");
    assert_within_tolerance(thumb.origin.x, px(260.8));
    assert_px(thumb.origin.y, px(-6.0));
    assert_px(thumb.size.width, px(16.0));

    // The track and the root are untouched by the value.
    let off = measure(cx, cell(&[]));
    assert_px(
        at(&records, "slider").size.width,
        at(&off, "slider").size.width,
    );
    assert_px(
        at(&records, "slider-track").size.width,
        at(&off, "slider-track").size.width,
    );
}

/// **`--width` moves the track, the indicator and the thumb's centre** — the
/// axis that separates this surface from `switch`'s vacuous one.
#[gpui::test]
fn the_width_axis_moves_the_track_and_the_thumbs_centre(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(cx, cell(&["--width", "200", "--value", "40"]));
    let wide = measure(cx, cell(&["--width", "600", "--value", "40"]));

    assert_px(at(&narrow, "slider").size.width, px(200.0));
    assert_px(at(&wide, "slider").size.width, px(600.0));
    assert_px(at(&narrow, "slider-track").size.width, px(196.0));
    assert_px(at(&wide, "slider-track").size.width, px(596.0));

    let narrow_thumb = at(&narrow, "slider-thumb");
    let wide_thumb = at(&wide, "slider-thumb");
    assert!(narrow_thumb.origin.x != wide_thumb.origin.x);
    // Both stay inside their own track: right edge never exceeds width - 8
    // (half the 16px thumb).
    assert!(f32::from(narrow_thumb.origin.x + narrow_thumb.size.width) <= 200.0);
    assert!(f32::from(wide_thumb.origin.x + wide_thumb.size.width) <= 600.0);
}

/// **`rounded-full` is `f32::MAX` on the track, the indicator and the
/// thumb** — the third surface this exact number is measured on.
#[gpui::test]
fn every_anchor_is_rounded_full(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for id in ["slider-track", "slider-indicator", "slider-thumb"] {
        assert_eq!(find(&records, id).radius, px(f32::MAX), "{id}");
    }
    // The root itself is not rounded — `slider.tsx`'s Control authors no
    // radius at all.
    assert_px(find(&records, "slider").radius, px(0.0));

    // Only the thumb carries a border.
    assert_px(find(&records, "slider").border_width, px(0.0));
    assert_px(find(&records, "slider-track").border_width, px(0.0));
    assert_px(find(&records, "slider-indicator").border_width, px(0.0));
    assert_px(find(&records, "slider-thumb").border_width, px(1.0));

    for id in ["slider", "slider-track", "slider-indicator", "slider-thumb"] {
        assert!(find(&records, id).visible, "{id}");
    }
}

/// `bg-input` on the track, `bg-primary` on the indicator, `bg-white` on the
/// thumb — the live reference's own colours, in dark.
#[gpui::test]
fn the_three_painted_anchors_take_the_references_colours(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "dark"]));

    assert_eq!(solid(&records, "slider-track"), Theme::DARK.input.value());
    assert_eq!(
        solid(&records, "slider-indicator"),
        Theme::DARK.primary.value(),
    );
    assert_eq!(
        solid(&records, "slider-thumb"),
        crowbar_ui::theme::Color::WHITE.value(),
    );
}

/// Neither anchor paints text, so the whole text group is absent on this
/// side — what makes `--content` vacuous rather than merely unexercised.
#[gpui::test]
fn no_anchor_on_this_surface_paints_text(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [vec![], vec!["--value", "40"], vec!["--content", "overflow"]] {
        let records = measure(cx, cell(&args));
        for id in ["slider", "slider-track", "slider-indicator", "slider-thumb"] {
            assert!(
                find(&records, id).text.is_none(),
                "{id} painted text on {args:?}",
            );
        }
    }
}

/// The `sm:` arm is the live one, and the base arm is a genuinely bigger
/// thumb — laid out rather than computed, so taffy is what answers.
///
/// **A narrower `--width` than the live cell's 668, deliberately**: a 668px
/// surface needs at least a 692px window (its own two 12px insets), which is
/// already past the 640px `sm` threshold — so the live width can *never*
/// reach the base arm at all. `200` leaves room for a sub-640 viewport
/// (`200 + 24 = 224` is the floor) while the formula being tested is the
/// same one at either width.
#[gpui::test]
fn the_base_breakpoint_lays_out_a_bigger_thumb(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let sm = measure(
        cx,
        a_cell(&["--surface", "slider", "--width", "200", "--value", "40"]),
    );
    let base = measure(
        cx,
        a_cell(&[
            "--surface",
            "slider",
            "--width",
            "200",
            "--value",
            "40",
            "--viewport-width",
            "300",
        ]),
    );

    assert_px(at(&sm, "slider-thumb").size.width, px(16.0));
    assert_px(at(&base, "slider-thumb").size.width, px(20.0));
    assert_px(at(&sm, "slider-thumb").origin.y, px(-6.0));
    assert_px(at(&base, "slider-thumb").origin.y, px(-8.0));
    assert!(at(&sm, "slider-thumb").origin.x != at(&base, "slider-thumb").origin.x);
}
