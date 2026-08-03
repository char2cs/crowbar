//! `--surface sidebar-peek`, laid out in a real window.
//!
//! Pins the one structural claim `sidebar_peek.rs`'s own module docs make
//! about `full_bleed`: that `Closed`/`Peeking`'s own `.absolute()` card, with
//! no `.relative()` ancestor of its own, resolves against the **window** —
//! the same claim `row_layout::fps_overlay` pins for that surface, and the
//! same `--width == --viewport-width` convention its own test establishes.

use super::{a_cell, assert_px, find, ids, measure};
use crowbar_ui::components::sidebar_peek;
use gpui::{Pixels, TestAppContext, px};

use crate::row_surface::{Cell, INSET_Y, RowSurface};

/// A `Docked` cell — the ordinary, inset convention every other surface
/// takes.
fn docked_cell(width: u16, args: &[&str]) -> Cell {
    let width = width.to_string();
    let mut line = vec!["--surface", "sidebar-peek", "--width", &width];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// A `Closed`/`Peeking` cell — `--width` and `--viewport-width` driven
/// equal, the `fps_overlay.rs` convention this surface's own module docs
/// point to.
fn fixed_cell(viewport: u16, args: &[&str]) -> Cell {
    let viewport = viewport.to_string();
    let mut line = vec![
        "--surface",
        "sidebar-peek",
        "--width",
        &viewport,
        "--viewport-width",
        &viewport,
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// **Every cell — docked, closed or peeking alike — carries exactly one
/// anchor.** No other anchor id ever appears, on any of the three states.
#[gpui::test]
fn every_state_carries_exactly_the_one_root_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    assert_eq!(ids(&measure(cx, docked_cell(294, &[]))), vec![sidebar_peek::ID_ROOT.to_owned()]);
    assert_eq!(
        ids(&measure(cx, fixed_cell(1200, &["--state", "closed"]))),
        vec![sidebar_peek::ID_ROOT.to_owned()],
    );
    assert_eq!(
        ids(&measure(cx, fixed_cell(1200, &["--state", "peeking"]))),
        vec![sidebar_peek::ID_ROOT.to_owned()],
    );
}

/// **Docked fills the column exactly** — `w_full h_full`, tracking both
/// `--width` and `--height`.
///
/// `origin.y` is [`INSET_Y`], not zero: `RowSurface`'s own harness applies
/// `pt(INSET_Y)` **unconditionally**, on every surface regardless of
/// `full_bleed` — that flag is documented as deciding the *horizontal* inset
/// only (`Surface::full_bleed`'s own doc comment). An earlier draft of this
/// test asserted zero and failed with "expected 0px, got 16px", which is the
/// harness's own always-on top inset, not a defect in `Docked`'s own
/// geometry.
#[gpui::test]
fn docked_fills_the_column(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (width, height) in [(294u16, 600u16), (340, 900)] {
        let records = measure(cx, docked_cell(width, &["--height", &height.to_string()]));
        let root = find(&records, sidebar_peek::ID_ROOT);
        assert_px(root.bounds.size.width, px(f32::from(width)));
        assert_px(root.bounds.size.height, px(f32::from(height)));
        assert_px(root.bounds.origin.x, px(0.0));
        assert_px(root.bounds.origin.y, px(INSET_Y));
    }
}

/// **Peeking sits exactly `PEEK_MARGIN` off the window's left edge, and
/// `PEEK_MARGIN` off both the top and bottom edges of the harness's own
/// content area, at the driven `--peek-width`.**
///
/// `origin.x` is bare [`sidebar_peek::PEEK_MARGIN`] — `full_bleed` really
/// does zero the *horizontal* inset, confirmed here rather than assumed.
/// `origin.y` is `INSET_Y + PEEK_MARGIN`, not bare `PEEK_MARGIN`: the
/// harness's own unconditional `pt(INSET_Y)` (see
/// [`docked_fills_the_column`]'s own doc comment) is the immediate parent's
/// own top edge, and this surface's `top(PEEK_MARGIN)` is measured from
/// *that* edge, not from the window's. `bottom_gap`, in contrast, **is**
/// bare `PEEK_MARGIN` — the immediate parent's own bottom edge already sits
/// at the window's true bottom (nothing pads it), so `bottom(PEEK_MARGIN)`
/// lands exactly `PEEK_MARGIN` off the window either way. An earlier draft
/// asserted bare `PEEK_MARGIN` for `origin.y` too and failed with "expected
/// 8px, got 24px" — `16 + 8`, the same harness inset this surface's own
/// `Docked` test already accounts for.
#[gpui::test]
fn peeking_docks_left_by_the_peek_margin(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = fixed_cell(1200, &["--state", "peeking", "--peek-width", "300"]);
    let window = RowSurface::window_size(&cell);
    let records = measure(cx, cell);
    let root = find(&records, sidebar_peek::ID_ROOT);

    assert_px(root.bounds.origin.x, sidebar_peek::PEEK_MARGIN);
    assert_px(root.bounds.size.width, px(300.0));
    let bottom_gap = window.height - (root.bounds.origin.y + root.bounds.size.height);
    assert_px(root.bounds.origin.y, px(INSET_Y) + sidebar_peek::PEEK_MARGIN);
    assert_px(bottom_gap, sidebar_peek::PEEK_MARGIN);
}

/// **Closed parks the card fully past the window's left edge** — its own
/// width plus `OFFSCREEN_GAP` beyond where `Peeking` rests, on the negative
/// side of `x = 0`.
///
/// **Mutation, run:** dropping the `- OFFSCREEN_GAP` term from
/// `SidebarPeek::edge_offset`'s `Closed` arm still parks the card
/// off-screen (any negative `x` clears the window, so `assert!(x < 0.0)`
/// alone would stay green) but by exactly `OFFSCREEN_GAP` (16px) less than
/// this test's own independently-computed expectation — confirmed by
/// actually running the mutation rather than reasoned from the formula:
/// "expected -308px, got -292px", a 16px gap. **A first draft of this
/// comment claimed the gap was `PEEK_MARGIN` (8px) instead — wrong, and
/// caught only by running the mutation rather than trusting the arithmetic
/// on paper.** The exact-value assertion below is what catches either
/// number; a bare inequality would catch neither.
#[gpui::test]
fn closed_parks_the_card_fully_past_the_edge(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = fixed_cell(1200, &["--state", "closed", "--peek-width", "300"]);
    let records = measure(cx, cell);
    let root = find(&records, sidebar_peek::ID_ROOT);

    let expected: Pixels = sidebar_peek::PEEK_MARGIN - px(300.0) - sidebar_peek::OFFSCREEN_GAP;
    assert_px(root.bounds.origin.x, expected);
    assert!(root.bounds.origin.x < px(0.0));
    // Entirely clear of the window: the card's own right edge is still
    // negative.
    assert!(root.bounds.origin.x + root.bounds.size.width < px(0.0));
}

/// **`--right` mirrors both states off the window's right edge instead.**
#[gpui::test]
fn right_docked_mirrors_off_the_right_edge(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = fixed_cell(1200, &["--state", "peeking", "--right", "--peek-width", "300"]);
    let window = RowSurface::window_size(&cell);
    let records = measure(cx, cell);
    let root = find(&records, sidebar_peek::ID_ROOT);

    let right_gap = window.width - (root.bounds.origin.x + root.bounds.size.width);
    assert_px(right_gap, sidebar_peek::PEEK_MARGIN);

    let closed = fixed_cell(1200, &["--state", "closed", "--right", "--peek-width", "300"]);
    let closed_window = RowSurface::window_size(&closed);
    let closed_records = measure(cx, closed);
    let closed_root = find(&closed_records, sidebar_peek::ID_ROOT);
    // Fully clear of the window on the right this time: the card's own
    // left edge is past the window's own right edge.
    assert!(closed_root.bounds.origin.x > closed_window.width);
}

/// **The card never exceeds `viewport - MAX_WIDTH_GAP`**, even when
/// `--peek-width` asks for more.
///
/// **Mutation, run:** dropping `- MAX_WIDTH_GAP` from `SidebarPeek::render`'s
/// own `.max_w(...)` call turns this red at "expected 384px, got 400px" —
/// the cap stops applying and the card grows to the bare viewport width.
#[gpui::test]
fn the_card_is_capped_at_the_viewport_minus_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = fixed_cell(400, &["--state", "peeking", "--peek-width", "2000"]);
    let records = measure(cx, cell);
    let root = find(&records, sidebar_peek::ID_ROOT);

    assert_px(root.bounds.size.width, px(400.0) - sidebar_peek::MAX_WIDTH_GAP);
}

/// **The content filler moves nothing** — the card's own box does not
/// depend on the opaque sidebar column it wraps, in any of the three
/// states.
#[gpui::test]
fn the_content_filler_does_not_move_the_card(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let empty = measure(cx, fixed_cell(1200, &["--state", "peeking"]));
    let stuffed = measure(cx, fixed_cell(1200, &["--state", "peeking", "--content-width", "5000"]));

    assert_px(
        find(&stuffed, sidebar_peek::ID_ROOT).bounds.size.width,
        find(&empty, sidebar_peek::ID_ROOT).bounds.size.width,
    );
    assert_px(
        find(&stuffed, sidebar_peek::ID_ROOT).bounds.origin.x,
        find(&empty, sidebar_peek::ID_ROOT).bounds.origin.x,
    );
}

/// **The theme axis paints the card, but moves no bound** — `bg`, `border`
/// and `radius` are real (unlike `sidebar-carousel`'s own paints-nothing
/// surface), but geometry is theme-independent.
#[gpui::test]
fn the_card_paints_a_real_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, fixed_cell(1200, &["--state", "peeking"]));
    let root = find(&records, sidebar_peek::ID_ROOT);

    assert!(matches!(root.background, crowbar_driver::Paint::Solid(_)));
    assert_px(root.border_width, sidebar_peek::BORDER_WIDTH);
    assert!(root.border_color.is_some());
    assert!(root.radius > px(0.0));
}
