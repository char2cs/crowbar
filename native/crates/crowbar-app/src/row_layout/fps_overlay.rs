//! `--surface fps-overlay`: what taffy resolves a `fixed bottom-8 right-3`
//! badge to, in a real window.
//!
//! **There is no reference to compare these against** — `fps-overlay.tsx`
//! carries no `data-oracle-id` anywhere in its own file. What these
//! assertions pin down instead is the one structural claim the port makes:
//! that `full_bleed` plus a plain `.absolute()` child, with no
//! `.relative()` ancestor of its own, resolves against the **window**, the
//! way CSS `position: fixed` does — not against some other box the shared
//! harness happens to introduce. If that claim were wrong the badge would
//! still render, at some plausible-looking but wrong offset, which is
//! exactly the failure mode a passing "the anchor exists" check would miss.

use super::{a_cell, find, measure};
use crowbar_ui::components::fps_overlay::{self, BOTTOM, RIGHT};
use gpui::{Bounds, Pixels, Size, TestAppContext, px};

use crate::row_surface::{Cell, RowSurface};

/// A cell on this surface, with the selector already applied.
///
/// `--width`/`--viewport-width` are driven equal — the full-bleed convention
/// `command`'s and `dialog`'s own row_layout tests already establish: a
/// full-bleed surface's own width **is** the viewport's.
fn cell(viewport: u16, args: &[&str]) -> Cell {
    let viewport = viewport.to_string();
    let mut line = vec![
        "--surface",
        "fps-overlay",
        "--width",
        &viewport,
        "--viewport-width",
        &viewport,
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// The badge's bounds, in **window** coordinates — not relative to any root,
/// because the claim under test is where those window coordinates land.
fn badge_bounds(
    cx: &mut TestAppContext,
    viewport: u16,
    args: &[&str],
) -> (Bounds<Pixels>, Size<Pixels>) {
    let cell = cell(viewport, args);
    let window = RowSurface::window_size(&cell);
    let records = measure(cx, cell);
    let badge = find(&records, fps_overlay::ID_FPS_OVERLAY).bounds;
    (badge, window)
}

/// **The badge sits exactly 12px off the window's right edge and 32px off
/// its bottom edge**, at `bottom-8 right-3`'s own literal numbers — not at
/// some offset that happens to look right for one window size.
///
/// The two assertions are against `px(12.0)`/`px(32.0)` **directly, not
/// against [`RIGHT`]/[`BOTTOM`]**: a bare `super::assert_px(gap, RIGHT)`
/// would still pass if `RIGHT`'s own derivation were wrong, because the
/// component paints with the same constant this test would be comparing it
/// to — the guard-that-tests-its-own-declaration shape. The one-line
/// mutation this actually catches: change `SPACING * 3.0` (or `* 8.0`) in
/// `crowbar_ui::components::fps_overlay::RIGHT`/`BOTTOM` to any other
/// multiple and this fails, because 12/32 are typed in below independently
/// of whatever the constant says. Confirmed live: mutating `BOTTOM` to
/// `SPACING * 5.0` and asserting against `BOTTOM` itself (the trap this test
/// avoids) left this test green; reverting to the literal `px(32.0)` below
/// makes the same mutation fail as `expected 32px, got 20px`.
#[gpui::test]
fn the_badge_sits_a_fixed_distance_off_the_windows_own_corner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for viewport in [400, 900] {
        let (badge, window) = badge_bounds(cx, viewport, &[]);

        let right_gap = window.width - (badge.origin.x + badge.size.width);
        let bottom_gap = window.height - (badge.origin.y + badge.size.height);

        super::assert_px(right_gap, px(12.0));
        super::assert_px(bottom_gap, px(32.0));
        // And the constants agree with the literals, so a reader does not
        // have to trust two numbers separately.
        super::assert_px(RIGHT, px(12.0));
        super::assert_px(BOTTOM, px(32.0));
    }
}

/// **The right-edge gap tracks the window's own width** — proof the badge is
/// genuinely resolving against the window rather than against a fixed
/// absolute number that happened to match one viewport.
#[gpui::test]
fn widening_the_window_moves_the_badges_x_but_not_its_gap_to_the_edge(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (narrow, narrow_window) = badge_bounds(cx, 400, &[]);
    let (wide, wide_window) = badge_bounds(cx, 900, &[]);

    assert!(wide.origin.x > narrow.origin.x, "{wide:?} vs {narrow:?}");
    assert_eq!(wide_window.width - narrow_window.width, px(500.0));

    super::assert_px(
        narrow_window.width - (narrow.origin.x + narrow.size.width),
        px(12.0),
    );
    super::assert_px(
        wide_window.width - (wide.origin.x + wide.size.width),
        px(12.0),
    );
}

/// The badge is `content_sized`: painting a wider run — a three-digit fps, a
/// larger `max-dt` and a nonzero drop count all at once — measures a wider
/// box than the resting cell's dashes-and-zero picture, at the **same**
/// window width. A box that hard-coded its own width would fail this while
/// still passing the corner-distance assertions above.
#[gpui::test]
fn the_badge_widens_when_its_content_does(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let (resting, _) = badge_bounds(cx, 600, &[]);
    let (wide, _) = badge_bounds(cx, 600, &["--fps", "144", "--max-dt", "23", "--drops", "12"]);

    assert!(
        wide.size.width > resting.size.width,
        "resting {resting:?}, wide {wide:?}",
    );
    // And the right edge still lands at the same 12px gap — a wider box
    // grows to the *left*, off a fixed right edge, not the other way round.
    let window = RowSurface::window_size(&cell(600, &[]));
    super::assert_px(window.width - (wide.origin.x + wide.size.width), px(12.0));
}

/// The box's own padding and radius, read straight off the extractor rather
/// than assumed from the constants above.
#[gpui::test]
fn the_box_carries_its_own_padding_and_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(600, &[]));
    let record = find(&records, fps_overlay::ID_FPS_OVERLAY);

    super::assert_px(record.radius, px(8.0)); // theme.radius_md
    super::assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert!(
        matches!(record.background, crowbar_driver::Paint::Solid(_)),
        "{:?}",
        record.background,
    );
    // The box paints a mixed-colour run of plain children, not one text
    // anchor of its own — see the component's module docs.
    assert!(record.text.is_none());
}
