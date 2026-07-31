//! `--surface callout-node`: what taffy resolves a callout to, in a real window,
//! against the captured reference at `/tmp/p3-ref-callout-node.json`.
//!
//! The reference is `720 × 68.58` over four anchors, and the thing worth
//! measuring here is that **none of the numbers come from the class list**: the
//! padding, radius and background are all `.crowbar-markdown-editor
//! .slate-callout`'s, and the utilities they beat are still written in the JSX.
//! A port that reverted to reading them would be wrong in five properties, and
//! the assertions below are stated against the stylesheet's values so it would
//! fail here rather than in a matrix run.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::callout_node;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "callout-node"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the callout's own box.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, callout_node::ID_CALLOUT, id)
}

/// The four anchors, in the order `prepaint` reaches them, and the root at the
/// origin.
///
/// The set matters as much as the geometry: `callout-node.tsx` renders all four
/// unconditionally, which is what would let this surface declare its set — and
/// it does **not** hold the Plate paragraph inside it, which belongs to
/// `ParagraphElement` and carries no anchor here.
#[gpui::test]
fn the_surface_records_its_own_four_anchors_and_nothing_else(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "720"]));

    assert_eq!(
        ids(&records),
        vec![
            "callout".to_owned(),
            "callout-row".to_owned(),
            "callout-emoji".to_owned(),
            "callout-content".to_owned(),
        ],
    );

    let root = at(&records, callout_node::ID_CALLOUT);
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
}

/// **The stylesheet's box, not the class list's.** Every one of these would be a
/// different number if the port had read `rounded-sm`, `p-4 pl-3` or `bg-muted`.
///
/// The reference: `radius 8`, `border.w 0`, and children inset by `16 / 13.6`.
#[gpui::test]
fn the_box_is_the_stylesheets_and_not_the_utilities(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "720"]));

    let record = find(&records, callout_node::ID_CALLOUT);
    // `--radius-md`, not `rounded-sm`'s 6.
    assert_px(record.radius, px(8.0));
    assert!(
        (f32::from(record.radius) - 6.0).abs() > 0.5,
        "not rounded-sm"
    );
    // The callout carries no `border` utility at all, so preflight's 0 stands.
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    // It paints no run of its own: its children are elements.
    assert!(record.text.is_none());

    // `padding: 0.85rem 1rem` puts the row at (16, 13.6). The inline axis is
    // exact; the block axis is **not**, and the difference is the engines':
    // `0.85rem` is 13.6, WebKit keeps it (the reference reads 13.59) and gpui
    // snaps padding to the device grid, landing on **13.5** at DPR 2. Δ 0.1,
    // inside the ±0.5 §5 allows — the same shape as v1.10's intrinsic ratio, and
    // recorded in `native/mapping/callout-node.md` rather than papered over.
    let row = at(&records, callout_node::ID_ROW);
    assert_px(row.origin.x, callout_node::PADDING_X);
    assert_within_tolerance(row.origin.y, callout_node::PADDING_Y);
    // ...and NOT `pl-3`'s 12, which is the class the JSX actually writes.
    assert!(
        (f32::from(row.origin.x) - 12.0).abs() > 0.5,
        "pl-3 must not survive",
    );
}

/// The emoji control's own box, and the two facts that make it interesting.
///
/// `24 × 32` — `sm:h-8` takes the height off `size-6` and leaves the width — and
/// `radius 10`, the primitive's **unmerged** `rounded-lg` on a control that is
/// `visible`. The second contradicts P3.1's recorded "no live Button is both
/// unmerged and visible", which is why it is asserted rather than only written
/// down.
#[gpui::test]
fn the_emoji_control_is_unmerged_and_not_square(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "720"]));

    let bounds = at(&records, callout_node::ID_EMOJI);
    assert_px(bounds.size.width, callout_node::EMOJI_WIDTH);
    assert_px(bounds.size.height, callout_node::EMOJI_HEIGHT);
    assert!(
        bounds.size.height > bounds.size.width,
        "sm:h-8 beats size-6 on the height only: {bounds:?}",
    );

    let record = find(&records, callout_node::ID_EMOJI);
    // `rounded-lg` = 10, the size variant's own — no call-site `rounded-*`.
    assert_px(record.radius, px(10.0));
    // Every button variant carries a bare `border`, transparent or not.
    assert_px(record.border_width, px(1.0));
    assert!(record.visible, "and it is on screen, unlike P3.1's two");
}

/// **The emoji box is authored, so it is not `line_sized`** — and the distance is
/// the evidence, exactly as `badge`'s and `kbd`'s are.
///
/// The control is the only anchor on this surface carrying a font, so a wrong
/// declaration here would land on the one row that has something to compare.
#[gpui::test]
fn the_only_text_anchor_is_not_line_sized(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "720"]));

    let record = find(&records, callout_node::ID_EMOJI);
    assert!(!record.line_sized, "v1.6: the height is authored");
    assert!(!record.content_sized, "and the width is authored too");

    let text = record.text.clone().expect("the control paints its emoji");
    let height = at(&records, callout_node::ID_EMOJI).size.height;
    assert!(
        (f32::from(height) - f32::from(text.font.line_height)).abs() > 0.5,
        "declaring line_sized would compare {height:?} against \
         {:?} and manufacture a delta",
        text.font.line_height,
    );

    // The other three anchors carry no run at all, so the question cannot arise
    // for them.
    for id in [
        callout_node::ID_CALLOUT,
        callout_node::ID_ROW,
        callout_node::ID_CONTENT,
    ] {
        assert!(find(&records, id).text.is_none(), "{id}");
    }
}

/// `gap-2` is the one Tailwind utility the stylesheet leaves alone, and it is
/// what puts the content box at `x 48` — `16 + 24 + 8`, which is what the
/// reference reads.
#[gpui::test]
fn the_row_gap_places_the_content_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "720"]));

    let emoji = at(&records, callout_node::ID_EMOJI);
    let content = at(&records, callout_node::ID_CONTENT);
    assert_px(
        content.origin.x,
        emoji.origin.x + emoji.size.width + callout_node::GAP,
    );
    assert_px(content.origin.x, px(48.0));
    // Both children sit on the same top edge.
    assert_px(content.origin.y, emoji.origin.y);
}

/// The callout is `w-full`, so `--width` reaches every anchor — and the inner
/// boxes give up exactly the padding and the gap.
#[gpui::test]
fn every_box_tracks_the_surface_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [480_u16, 720] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        let root = at(&records, callout_node::ID_CALLOUT);
        assert_px(root.size.width, px(f32::from(width)));

        let inner = f32::from(width) - 2.0 * f32::from(callout_node::PADDING_X);
        assert_px(at(&records, callout_node::ID_ROW).size.width, px(inner));
        assert_px(
            at(&records, callout_node::ID_CONTENT).size.width,
            px(inner - f32::from(callout_node::EMOJI_WIDTH) - f32::from(callout_node::GAP)),
        );
    }
}

/// The callout's height is its padding plus its content's, which is what makes
/// `--content` a real axis rather than one that only moves a string.
#[gpui::test]
fn the_height_is_the_padding_plus_the_content(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "720"]));

    let root = at(&records, callout_node::ID_CALLOUT);
    let row = at(&records, callout_node::ID_ROW);
    // Against the padding the engine actually resolved — see
    // `the_box_is_the_stylesheets_and_not_the_utilities` for why that is 13.5
    // here and 13.6 in the stylesheet. Written as the recorded inset rather than
    // as the constant so this measures the *composition* and not the snap twice.
    let inset = row.origin.y;
    assert_px(root.size.height, row.size.height + inset + inset);
    assert_within_tolerance(inset, callout_node::PADDING_Y);
}

/// `hover` is the surface's one real state flag, and it moves a **compared**
/// field: the control's own `bg`.
#[gpui::test]
fn hover_moves_the_controls_background_and_nothing_else(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&["--width", "720"]));
    let hovered = measure(cx, cell(&["--width", "720", "--flags", "hover"]));

    let resting_bg = find(&resting, callout_node::ID_EMOJI).background;
    let hovered_bg = find(&hovered, callout_node::ID_EMOJI).background;
    assert_ne!(
        resting_bg, hovered_bg,
        "hover:bg-muted-foreground/15 is the only interaction rule here",
    );

    // The geometry is untouched, which is what makes the flag a colour change
    // rather than a different picture.
    assert_eq!(ids(&resting), ids(&hovered));
    for id in [
        callout_node::ID_CALLOUT,
        callout_node::ID_ROW,
        callout_node::ID_EMOJI,
        callout_node::ID_CONTENT,
    ] {
        assert_eq!(at(&resting, id), at(&hovered, id), "{id}");
    }
}

/// §8.3's `empty`: the paragraph keeps its line box, so `callout-content`
/// **collapses rather than vanishing** — all four anchors survive.
#[gpui::test]
fn the_empty_cell_keeps_all_four_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "720", "--flags", "empty"]));

    assert_eq!(ids(&records).len(), 4, "{:?}", ids(&records));
    // The control still paints its emoji: only the body went away.
    assert!(find(&records, callout_node::ID_EMOJI).text.is_some());
    let content = at(&records, callout_node::ID_CONTENT);
    assert!(
        f32::from(content.size.height) > 0.0,
        "an empty paragraph still has a line box and a trailing gap: {content:?}",
    );
}
