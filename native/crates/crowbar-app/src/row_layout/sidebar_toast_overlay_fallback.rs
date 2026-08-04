//! `--surface sidebar-toast-overlay-fallback`: what taffy resolves the
//! `Toast.Portal`'d, fixed-corner viewport to, in a real window.
//!
//! **There is no reference to compare against** — see
//! `native/mapping/sidebar-toast-overlay.md`. What these assertions pin
//! down, `fps_overlay`'s own row-layout test one door over: that
//! `full_bleed` plus a plain `.absolute()` child with no `.relative()`
//! ancestor resolves against the **window**, not against some other box the
//! shared harness happens to introduce — and that this surface, unlike its
//! inline sibling, never windows its own toast list.

use super::{a_cell, find, measure};
use crowbar_ui::components::sidebar_toast_overlay::{self, SIDEBAR_TOAST_LIMIT};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::{Cell, RowSurface};

/// A cell on this surface. `--width`/`--viewport-width` driven equal — the
/// full-bleed convention `fps_overlay`'s own `row_layout` test already
/// establishes.
fn cell(viewport: u16, args: &[&str]) -> Cell {
    let viewport = viewport.to_string();
    let mut line = vec![
        "--surface",
        "sidebar-toast-overlay-fallback",
        "--width",
        &viewport,
        "--viewport-width",
        &viewport,
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// The viewport's own bounds, in **window** coordinates, plus the window's
/// own size.
fn viewport_bounds(
    cx: &mut TestAppContext,
    viewport: u16,
    args: &[&str],
) -> (Bounds<Pixels>, gpui::Size<Pixels>) {
    let built = cell(viewport, args);
    let window = RowSurface::window_size(&built);
    let records = measure(cx, built);
    let bounds = find(&records, sidebar_toast_overlay::ID_VIEWPORT_FALLBACK).bounds;
    (bounds, window)
}

/// **The viewport sits a fixed distance off the window's own left corner**
/// at `--side left` (the default) — not against some other ancestor the
/// shared harness happens to introduce.
#[gpui::test]
fn left_docks_a_fixed_distance_off_the_windows_own_corner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for viewport in [400, 900] {
        let (bounds, window) = viewport_bounds(cx, viewport, &[]);
        let left_gap = bounds.origin.x;
        let bottom_gap = window.height - (bounds.origin.y + bounds.size.height);
        super::assert_px(left_gap, sidebar_toast_overlay::FALLBACK_INSET);
        super::assert_px(bottom_gap, sidebar_toast_overlay::FALLBACK_INSET);
        // Typed independently of the constant too, `fps_overlay`'s own
        // guard-against-self-referential-mutation shape.
        super::assert_px(left_gap, px(16.0));
        super::assert_px(bottom_gap, px(16.0));
    }
}

/// `--side right` docks off the **right** edge instead — proof the side is
/// genuinely read, not a constant left-over from the default.
#[gpui::test]
fn right_docks_off_the_windows_right_corner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (bounds, window) = viewport_bounds(cx, 900, &["--side", "right"]);
    let right_gap = window.width - (bounds.origin.x + bounds.size.width);
    super::assert_px(right_gap, px(16.0));
    // And it did **not** also dock left — the two are mutually exclusive.
    assert!(f32::from(bounds.origin.x) > 16.5, "{bounds:?}");
}

/// **The gap tracks the window's own width** at `--side right` — proof the
/// viewport is resolving against the window rather than a fixed absolute
/// number that happened to match one viewport, `fps_overlay`'s own test.
#[gpui::test]
fn widening_the_window_moves_the_viewport_but_not_its_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (narrow, narrow_window) = viewport_bounds(cx, 400, &["--side", "right"]);
    let (wide, wide_window) = viewport_bounds(cx, 900, &["--side", "right"]);

    assert!(wide.origin.x > narrow.origin.x, "{wide:?} vs {narrow:?}");
    assert_eq!(wide_window.width - narrow_window.width, px(500.0));
    super::assert_px(narrow_window.width - (narrow.origin.x + narrow.size.width), px(16.0));
    super::assert_px(wide_window.width - (wide.origin.x + wide.size.width), px(16.0));
}

/// `w-72`: the viewport's own width is authored, not driven by `--width` —
/// the axis table's own claim.
#[gpui::test]
fn the_viewport_width_is_authored_not_driven(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (narrow, _) = viewport_bounds(cx, 400, &[]);
    let (wide, _) = viewport_bounds(cx, 900, &[]);
    assert_eq!(narrow.size.width, sidebar_toast_overlay::FALLBACK_WIDTH);
    assert_eq!(wide.size.width, sidebar_toast_overlay::FALLBACK_WIDTH);
    assert_eq!(narrow.size.width, px(288.0));
}

/// **Uncapped**: `--toasts outage` renders all five fixture toasts here,
/// where the sibling (inline) surface renders only the three
/// `select_visible` keeps.
#[gpui::test]
fn outage_is_not_windowed_on_this_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let single = measure(cx, cell(900, &["--toasts", "single"]));
    let outage = measure(cx, cell(900, &["--toasts", "outage"]));

    let single_height = find(&single, sidebar_toast_overlay::ID_VIEWPORT_FALLBACK)
        .bounds
        .size
        .height;
    let outage_height = find(&outage, sidebar_toast_overlay::ID_VIEWPORT_FALLBACK)
        .bounds
        .size
        .height;

    // 5 items against 1 — a far larger jump than the 3-item cap would allow,
    // which is the point: if this surface windowed too, the ratio would top
    // out around 3x a one-item box plus its own gaps, not 5x.
    assert!(
        f32::from(outage_height) > f32::from(single_height) * 3.5,
        "outage {outage_height:?} against single {single_height:?} — looks capped",
    );
    let _ = SIDEBAR_TOAST_LIMIT;
}

/// `empty` renders no children at all — the smallest this surface's box
/// ever is, `flex-col`'s own zero-content collapse (no padding is authored
/// on this viewport, unlike the inline sibling's `p-2`).
#[gpui::test]
fn empty_collapses_to_zero(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let (bounds, _) = viewport_bounds(cx, 900, &["--flags", "empty"]);
    assert!(f32::from(bounds.size.height) < 1.0, "{bounds:?}");
}
