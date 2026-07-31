//! `--surface inline-error`: what taffy resolves the retry panel to, in a real
//! window.
//!
//! **There is no reference to compare against** — the store state that renders
//! this panel cannot be reached, for the reason
//! `crowbar_ui::components::inline_error` measures. So these assertions are
//! against the probe's values (`294 × 128`, `p-6`, `gap-2`, three centred runs
//! and a `sm:h-7` control) and against the component's own composition, which is
//! what a layout test can still say when the oracle cannot.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::button::Size;
use crowbar_ui::components::git_status_row::{BREAKPOINT_SM, Breakpoint};
use crowbar_ui::components::inline_error;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, at the sidebar's own 294px unless told otherwise.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "inline-error", "--width", "294"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the panel itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, inline_error::ID_PANEL, id)
}

/// Five anchors in a dev build, root at the origin, in `inline-error.tsx`'s own
/// order.
#[gpui::test]
fn the_dev_build_records_five_anchors_in_source_order(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec![
            "inline-error".to_owned(),
            "inline-error-glyph".to_owned(),
            "inline-error-title".to_owned(),
            "inline-error-detail".to_owned(),
            "inline-error-retry".to_owned(),
        ],
    );

    let root = at(&records, inline_error::ID_PANEL);
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(294.0));
}

/// **`empty` is the production build**, and it drops an anchor rather than
/// changing a field — which is the loudest failure the differ has, and exactly
/// why this surface declares no anchor set.
#[gpui::test]
fn the_production_build_drops_the_detail_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));

    assert_eq!(
        ids(&records),
        vec![
            "inline-error".to_owned(),
            "inline-error-glyph".to_owned(),
            "inline-error-title".to_owned(),
            "inline-error-retry".to_owned(),
        ],
    );
    assert!(
        !ids(&records).contains(&"inline-error-detail".to_owned()),
        "the detail line is behind import.meta.env.DEV",
    );
}

/// `p-6` and `gap-2`: the children start one padding in and are spaced by the
/// gap, down the column.
#[gpui::test]
fn the_column_is_the_padding_and_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let glyph = at(&records, inline_error::ID_GLYPH);
    assert_px(glyph.origin.y, inline_error::PADDING);

    let mut previous = glyph;
    for id in [
        inline_error::ID_TITLE,
        inline_error::ID_DETAIL,
        inline_error::ID_RETRY,
    ] {
        let next = at(&records, id);
        let expected = previous.origin.y + previous.size.height + inline_error::GAP;
        // The retry control adds its own `mt-1` on top of the flex gap.
        let expected = if id == inline_error::ID_RETRY {
            expected + inline_error::RETRY_MARGIN_TOP
        } else {
            expected
        };
        assert_px(next.origin.y, expected);
        previous = next;
    }
}

/// `items-center` on a flex column leaves the cross axis unstretched, so every
/// child is **narrower than the panel** and centred in it — which is what the
/// `content_sized` declaration claims.
#[gpui::test]
fn every_child_is_content_sized_and_centred(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let panel = at(&records, inline_error::ID_PANEL);
    for id in [
        inline_error::ID_GLYPH,
        inline_error::ID_TITLE,
        inline_error::ID_DETAIL,
        inline_error::ID_RETRY,
    ] {
        let child = at(&records, id);
        assert!(
            child.size.width < panel.size.width,
            "{id} was stretched: {child:?} in {panel:?}",
        );
        // Centred: the left inset equals the right one.
        let left = f32::from(child.origin.x);
        let right = f32::from(panel.size.width) - left - f32::from(child.size.width);
        assert!((left - right).abs() < 0.6, "{id}: {left} against {right}");
        assert!(find(&records, id).content_sized, "{id}");
    }
}

/// The three runs' boxes **are** their line boxes; the retry control's is not.
///
/// The control is the paired control the same way `kbd` is `label`'s: without it
/// "the box equals the line box" would be a property every element satisfies and
/// the assertion would prove nothing.
#[gpui::test]
fn the_three_runs_are_line_sized_and_the_control_is_not(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for id in [
        inline_error::ID_GLYPH,
        inline_error::ID_TITLE,
        inline_error::ID_DETAIL,
    ] {
        let record = find(&records, id);
        assert!(record.line_sized, "{id} declares it");
        let text = record
            .text
            .clone()
            .unwrap_or_else(|| panic!("{id} paints a run"));
        let height = at(&records, id).size.height;
        assert!(
            (f32::from(height) - f32::from(text.font.line_height)).abs() <= 0.5,
            "{id}: {height:?} against {:?}",
            text.font.line_height,
        );
    }

    let control = find(&records, inline_error::ID_RETRY);
    assert!(!control.line_sized);
    let text = control.text.clone().expect("the control paints its label");
    let height = at(&records, inline_error::ID_RETRY).size.height;
    assert!(
        (f32::from(height) - f32::from(text.font.line_height)).abs() > 0.5,
        "the control's height is authored: {height:?} against {:?}",
        text.font.line_height,
    );
}

/// The retry control is `h-8 sm:h-7`, so `--viewport-width` is a real axis — and
/// it is the **only** thing on this surface that moves at the breakpoint.
#[gpui::test]
fn the_control_height_moves_at_the_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "1714"]));
    let narrow_width = 639_u16;
    assert!(f32::from(narrow_width) < BREAKPOINT_SM);
    let narrow = measure(cx, cell(&["--viewport-width", &narrow_width.to_string()]));

    assert_px(
        at(&wide, inline_error::ID_RETRY).size.height,
        Size::Sm.extent(Breakpoint::Sm),
    );
    assert_px(
        at(&narrow, inline_error::ID_RETRY).size.height,
        Size::Sm.extent(Breakpoint::Base),
    );
    assert!(
        at(&narrow, inline_error::ID_RETRY).size.height
            > at(&wide, inline_error::ID_RETRY).size.height,
        "h-8 is taller than sm:h-7",
    );

    // The glyph is unaffected: nothing else here carries an `sm:` rule.
    assert_px(
        at(&narrow, inline_error::ID_GLYPH).size.height,
        at(&wide, inline_error::ID_GLYPH).size.height,
    );
}

/// `--content` moves the detail run's width, strictly, and never introduces a
/// break — a wrapped run is outside what the contract compares.
#[gpui::test]
fn the_detail_width_tracks_the_content_without_wrapping(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut widths = Vec::new();
    let mut heights = Vec::new();
    for content in ["short", "normal", "overflow"] {
        let records = measure(cx, cell(&["--content", content]));
        let bounds = at(&records, inline_error::ID_DETAIL);
        widths.push(f32::from(bounds.size.width));
        heights.push(f32::from(bounds.size.height));
    }
    assert!(widths[0] < widths[1], "{widths:?}");
    assert!(widths[1] < widths[2], "{widths:?}");
    // One line at every length: a second would double the height.
    assert!(
        (heights[0] - heights[2]).abs() < 0.5,
        "the overflow run wrapped: {heights:?}",
    );
}

/// The panel is `flex-1`, so it takes the width it is given and re-centres its
/// column inside it.
#[gpui::test]
fn the_panel_stretches_and_the_column_recentres(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(cx, a_cell(&["--surface", "inline-error", "--width", "294"]));
    let wide = measure(cx, a_cell(&["--surface", "inline-error", "--width", "420"]));

    assert_px(at(&narrow, inline_error::ID_PANEL).size.width, px(294.0));
    assert_px(at(&wide, inline_error::ID_PANEL).size.width, px(420.0));

    // Same run, same box; only its inset moved.
    let narrow_title = at(&narrow, inline_error::ID_TITLE);
    let wide_title = at(&wide, inline_error::ID_TITLE);
    assert_px(narrow_title.size.width, wide_title.size.width);
    assert!(wide_title.origin.x > narrow_title.origin.x);
}

/// **A run wider than its box wraps here and would not in `WebKit`** — the reason
/// this surface's `overflow` cell is the longest run that still fits rather than
/// one that genuinely overruns.
///
/// The panel's runs carry no `truncate` and no `overflow-hidden`, so CSS with
/// `overflow-wrap: normal` and no break opportunity lets a run spill on one
/// line. gpui breaks it instead. That is a whole line box of disagreement on
/// `bounds.h`, which the contract compares at ±0.5 — so such a cell could never
/// converge, and driving one would report a delta whose cause is the engines
/// rather than the port.
///
/// Driven through `--title`, which takes an arbitrary string, so this measures a
/// **real second rendering** rather than restating the fixture. The string has no
/// space and no hyphen, so neither engine has a break opportunity to take —
/// which is exactly the case where the two disagree.
#[gpui::test]
fn an_unbreakable_run_wider_than_the_box_wraps_here_and_would_not_in_webkit(
    cx: &mut TestAppContext,
) {
    crowbar_driver::leak_checked!(cx);

    let short = measure(cx, cell(&["--title", "Failed"]));
    let one_line = at(&short, inline_error::ID_TITLE).size.height;

    let overrun = measure(
        cx,
        cell(&[
            "--title",
            "FailedtoloadtheworkspacetreefromtheentitycachebecauseIndexedDBrefused",
        ]),
    );
    let wrapped = at(&overrun, inline_error::ID_TITLE).size.height;

    assert!(
        f32::from(wrapped) > 1.5 * f32::from(one_line),
        "gpui wrapped an unbreakable run: {wrapped:?} against {one_line:?}",
    );
    // And it stayed inside the panel rather than overflowing it, which is the
    // half `WebKit` does differently.
    let panel = at(&overrun, inline_error::ID_PANEL);
    let content_box = f32::from(panel.size.width) - 2.0 * f32::from(inline_error::PADDING);
    assert!(
        f32::from(at(&overrun, inline_error::ID_TITLE).size.width) <= content_box + 0.5,
        "gpui fits the run to the box; `WebKit` would let it spill",
    );

    // The shipped `overflow` fixture is on the safe side of that line, which is
    // what keeps all three content cells comparable.
    let fits = measure(cx, cell(&["--content", "overflow"]));
    assert_px(
        at(&fits, inline_error::ID_DETAIL).size.height,
        at(
            &measure(cx, cell(&["--content", "short"])),
            inline_error::ID_DETAIL,
        )
        .size
        .height,
    );
}

/// **`opacity-50` on the glyph does not hide it**, and this goes through the
/// driver's own v1.7 opacity term rather than restating the constant.
///
/// v1.7 makes `visible` false at *zero* opacity and only there. The glyph is the
/// only element in the port that carries a non-zero, non-one opacity, so it is
/// the only place that term is exercised at all — and it must come back `true`
/// on both sides. A driver that treated "has an opacity" as "is invisible" would
/// fail here; so would one that forgot to descend into the element.
#[gpui::test]
fn the_half_transparent_glyph_is_still_visible(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for id in [
        inline_error::ID_PANEL,
        inline_error::ID_GLYPH,
        inline_error::ID_TITLE,
        inline_error::ID_DETAIL,
        inline_error::ID_RETRY,
    ] {
        assert!(find(&records, id).visible, "{id}");
    }

    // ...and the glyph is genuinely translucent, so the assertions above are
    // about a non-opaque element rather than an opaque one. A **compile-time**
    // guard rather than a runtime `assert!`: the operand is a constant, so at
    // runtime it can never fail and would be one of the vacuous guards this
    // project has already had to delete once. In a `const` block it fails the
    // *build* if anyone moves the value to 0 or 1, which is the only way this
    // claim can be wrong.
    const {
        assert!(inline_error::GLYPH_OPACITY > 0.0);
        assert!(inline_error::GLYPH_OPACITY < 1.0);
    }
}
