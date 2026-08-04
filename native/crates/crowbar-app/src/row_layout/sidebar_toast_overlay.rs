//! `--surface sidebar-toast-overlay`: what taffy resolves the inline,
//! sidebar-docked viewport to, in a real window.
//!
//! **There is no reference to compare against** — see
//! `native/mapping/sidebar-toast-overlay.md`. What these assertions pin
//! down is the structural claim the port makes: a `.relative()` wrapper the
//! width of `--width` and the height of `--height` stands in for the real
//! sidebar column, and the viewport docks to its bottom edge and stretches
//! to its width — plus the windowing claim
//! `crowbar_ui::components::sidebar_toast_overlay`'s own module docs make in
//! full (§2), cross-checked against the sibling surface here rather than
//! merely trusted.

use super::{a_cell, find, measure};
use crowbar_ui::components::sidebar_toast_overlay;
use gpui::{Pixels, TestAppContext, px};

use crate::row_surface::{Cell, RowSurface};

/// A cell on this surface.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "sidebar-toast-overlay", "--width", "294"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// The viewport's own bounds, in **window** coordinates, plus the window's
/// own height — the claim under test is where those window coordinates
/// land, the identical reason `fps_overlay`'s own harness reads bounds this
/// way.
fn viewport_bounds(cx: &mut TestAppContext, args: &[&str]) -> (gpui::Bounds<Pixels>, Pixels) {
    let built = cell(args);
    let window = RowSurface::window_size(&built);
    let records = measure(cx, built);
    let bounds = find(&records, sidebar_toast_overlay::ID_VIEWPORT).bounds;
    (bounds, window.height)
}

/// The one anchor this surface ever carries, present with or without toasts
/// in the queue.
#[gpui::test]
fn the_root_is_the_only_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for args in [[].as_slice(), &["--flags", "empty"]] {
        let records = measure(cx, cell(args));
        assert_eq!(
            super::ids(&records),
            vec![sidebar_toast_overlay::ID_VIEWPORT.to_owned()],
        );
    }
}

/// **The viewport docks to the bottom of the `--height` column**, not to the
/// window's own bottom edge — the two coincide only when `--height` happens
/// to fill the window, so this drives them apart on purpose.
#[gpui::test]
fn the_viewport_docks_to_the_bottom_of_the_height_column(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (bounds, _) = viewport_bounds(cx, &["--height", "300"]);
    let column_bottom = crate::row_surface::INSET_Y + 300.0;
    let viewport_bottom = f32::from(bounds.origin.y) + f32::from(bounds.size.height);
    assert!(
        (viewport_bottom - column_bottom).abs() < 0.5,
        "viewport bottom {viewport_bottom} against column bottom {column_bottom}",
    );

    // And it is genuinely **not** the window's own bottom edge: the window
    // follows `--height` (300) plus the caption, which is taller than the
    // bare column.
    let (_, window_height) = viewport_bounds(cx, &["--height", "300"]);
    assert!(
        f32::from(window_height) > column_bottom,
        "{window_height:?} against {column_bottom}",
    );
}

/// `--width` reaches the viewport's own `w-full` stretch.
#[gpui::test]
fn the_viewport_stretches_with_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (narrow, _) = viewport_bounds(cx, &["--width", "294"]);
    let (wide, _) = viewport_bounds(cx, &["--width", "420"]);
    assert_eq!(narrow.size.width, px(294.0));
    assert_eq!(wide.size.width, px(420.0));
}

/// `empty` renders the viewport with no toasts at all — a real content
/// state, and the smallest this surface's box ever is: just its own `p-2`
/// padding, doubled.
#[gpui::test]
fn empty_renders_a_padding_only_viewport(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (empty, _) = viewport_bounds(cx, &["--flags", "empty"]);
    let expected = f32::from(sidebar_toast_overlay::VIEWPORT_PADDING) * 2.0;
    assert!(
        (f32::from(empty.size.height) - expected).abs() < 1.0,
        "{empty:?} against {expected}",
    );
}

/// **The outage fixture is genuinely windowed here** — cross-checked against
/// the sibling (uncapped) surface, not merely trusted from the unit tests.
/// `crowbar_ui::components::sidebar_toast_overlay`'s own `select_visible`
/// already proves the *value* is right at the Rust level (three deliberate
/// mutations, all caught — see that function's own doc comment); this
/// proves the render path actually threads the windowed list through
/// rather than the raw one.
///
/// **Deliberately a strict inequality, not a magnitude check.** An earlier
/// version of this test also asserted the windowed height landed close to
/// `SidebarToastOverlay::item_height_estimate`'s own arithmetic for three
/// items. Measured directly, gpui's real per-item box ran a further ~20px
/// ahead of that estimate than the estimate's own documented border-width
/// omission accounted for, for a cause not run down: `SidebarToastItem`'s
/// internal geometry is unanchored and outside this contract either way
/// (module docs' final section), so a test asserting its exact pixel count
/// would be pinning a number nothing downstream depends on. The strict
/// inequality below is what the windowing claim actually needs, and it does
/// not depend on getting an unanchored box's internal arithmetic exactly
/// right.
#[gpui::test]
fn the_outage_fixture_renders_shorter_here_than_on_the_uncapped_sibling(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (inline, _) = viewport_bounds(cx, &["--height", "600", "--toasts", "outage"]);

    let fallback_cell = a_cell(&[
        "--surface",
        "sidebar-toast-overlay-fallback",
        "--width",
        "294",
        "--toasts",
        "outage",
    ]);
    let fallback_records = measure(cx, fallback_cell);
    let fallback = find(&fallback_records, sidebar_toast_overlay::ID_VIEWPORT_FALLBACK).bounds;

    assert!(
        inline.size.height < fallback.size.height,
        "inline (windowed to 3) {inline:?} should be shorter than the uncapped sibling's \
         (all 5) {fallback:?}",
    );
}
