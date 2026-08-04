//! `--surface placeholder-row-actions`: what taffy resolves the reconstructed
//! reason plus Retry/Detach… pair to, in a real window.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::primitives::button::Size;
use crowbar_ui::surfaces::rows::git_status_row::{BREAKPOINT_SM, Breakpoint};
use crowbar_ui::surfaces::rows::placeholder_row_actions;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, at the detail wrapper's own real 262px
/// (`workspace-tree-item.tsx`'s `mx-1.5 px-2.5` around the 294px sidebar —
/// `native/mapping/placeholder-row-actions.md`'s own arithmetic).
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "placeholder-row-actions", "--width", "262"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to this surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, placeholder_row_actions::ID_PANEL, id)
}

/// `--held true` (the default): five anchors, root at the origin,
/// `placeholder-row-actions.tsx`'s own source order.
#[gpui::test]
fn held_renders_five_anchors_in_source_order(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec![
            "placeholder-row-actions".to_owned(),
            "placeholder-row-actions-reason".to_owned(),
            "placeholder-row-actions-actions".to_owned(),
            "placeholder-row-actions-retry".to_owned(),
            "placeholder-row-actions-detach".to_owned(),
        ],
    );

    let root = at(&records, placeholder_row_actions::ID_PANEL);
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(262.0));
}

/// `--held false` drops the Detach… anchor — the loudest failure the differ
/// has, the exact shape `inline-error`'s dev-only detail line takes.
#[gpui::test]
fn unheld_drops_the_detach_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--held", "false"]));

    assert_eq!(
        ids(&records),
        vec![
            "placeholder-row-actions".to_owned(),
            "placeholder-row-actions-reason".to_owned(),
            "placeholder-row-actions-actions".to_owned(),
            "placeholder-row-actions-retry".to_owned(),
        ],
    );
    assert!(
        !ids(&records).contains(&"placeholder-row-actions-detach".to_owned()),
        "the Detach… button is not rendered when heldByPath is unset",
    );
}

/// `gap-1.5` down the column: the action row starts one gap below the
/// reason's own (wrapped) bottom edge.
#[gpui::test]
fn the_column_is_the_gap_below_the_wrapped_reason(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let reason = at(&records, placeholder_row_actions::ID_REASON);
    let actions = at(&records, placeholder_row_actions::ID_ACTIONS);
    assert_px(reason.origin.y, px(0.0));
    assert_px(
        actions.origin.y,
        reason.origin.y + reason.size.height + placeholder_row_actions::GAP,
    );

    // The held reason really does wrap to more than one line at this width —
    // the module docs' own claim, pinned rather than merely asserted.
    let one_line = find(&records, placeholder_row_actions::ID_REASON)
        .text
        .as_ref()
        .map(|t| t.font.line_height)
        .expect("the reason paints a run");
    assert!(
        f32::from(reason.size.height) > f32::from(one_line) * 1.5,
        "the held reason should wrap to several lines: {reason:?} against a {one_line:?} line box",
    );
}

/// `justify-end gap-1.5`: the two buttons are flush with the row's right
/// edge, `gap-1.5` apart, Retry first.
#[gpui::test]
fn the_actions_row_is_right_justified_with_the_gap_between(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let actions = at(&records, placeholder_row_actions::ID_ACTIONS);
    let retry = at(&records, placeholder_row_actions::ID_RETRY);
    let detach = at(&records, placeholder_row_actions::ID_DETACH);

    // Detach… is flush with the row's right edge.
    assert_px(
        detach.origin.x + detach.size.width,
        actions.origin.x + actions.size.width,
    );
    assert_px(
        detach.origin.x,
        retry.origin.x + retry.size.width + placeholder_row_actions::GAP,
    );
    assert!(retry.origin.x > actions.origin.x, "Retry is not flush with the left edge");
}

/// Both buttons declare `content_sized` and paint their own labels — the
/// `inline-error` retry-control shape, twice.
#[gpui::test]
fn both_buttons_are_content_sized_and_paint_their_labels(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let retry = find(&records, placeholder_row_actions::ID_RETRY);
    assert!(retry.content_sized);
    assert!(!retry.line_sized);
    assert_eq!(retry.text.as_ref().expect("paints").content, "Retry");

    let detach = find(&records, placeholder_row_actions::ID_DETACH);
    assert!(detach.content_sized);
    assert!(!detach.line_sized);
    assert_eq!(detach.text.as_ref().expect("paints").content, "Detach…");

    // Different labels, different colours — the two variants must not read
    // as the same button.
    assert_ne!(
        at(&records, placeholder_row_actions::ID_RETRY).size.width,
        at(&records, placeholder_row_actions::ID_DETACH).size.width,
    );
    assert_ne!(retry.text.as_ref().unwrap().color, detach.text.as_ref().unwrap().color);
}

/// Both buttons' height moves at the `sm` breakpoint, in lockstep — the axis
/// `inline-error`'s own composed retry control carries.
#[gpui::test]
fn both_button_heights_move_at_the_breakpoint_together(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "1714"]));
    let narrow_width = 639_u16;
    assert!(f32::from(narrow_width) < BREAKPOINT_SM);
    let narrow = measure(cx, cell(&["--viewport-width", &narrow_width.to_string()]));

    assert_px(
        at(&wide, placeholder_row_actions::ID_RETRY).size.height,
        Size::Sm.extent(Breakpoint::Sm),
    );
    assert_px(
        at(&narrow, placeholder_row_actions::ID_RETRY).size.height,
        Size::Sm.extent(Breakpoint::Base),
    );
    assert_px(
        at(&narrow, placeholder_row_actions::ID_RETRY).size.height,
        at(&narrow, placeholder_row_actions::ID_DETACH).size.height,
    );
}

/// `--content` moves the interpolated branch strictly, in both the held and
/// unheld reason arms, without ever collapsing two lengths to one string.
#[gpui::test]
fn content_moves_the_interpolated_branch_in_both_arms(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for held in ["true", "false"] {
        let short = measure(cx, cell(&["--held", held, "--content", "short"]));
        let overflow = measure(cx, cell(&["--held", held, "--content", "overflow"]));
        let short_len = find(&short, placeholder_row_actions::ID_REASON)
            .text
            .expect("paints")
            .content
            .len();
        let overflow_len = find(&overflow, placeholder_row_actions::ID_REASON)
            .text
            .expect("paints")
            .content
            .len();
        assert!(overflow_len > short_len, "held={held}");
    }
}

/// The panel stretches with `--width`, and the action row re-justifies to
/// the new right edge.
#[gpui::test]
fn the_panel_stretches_and_the_action_row_rejustifies(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(cx, cell(&[]));
    let wide = measure(
        cx,
        a_cell(&["--surface", "placeholder-row-actions", "--width", "380"]),
    );

    assert_px(at(&narrow, placeholder_row_actions::ID_PANEL).size.width, px(262.0));
    assert_px(at(&wide, placeholder_row_actions::ID_PANEL).size.width, px(380.0));

    let narrow_detach = at(&narrow, placeholder_row_actions::ID_DETACH);
    let wide_detach = at(&wide, placeholder_row_actions::ID_DETACH);
    // Same button, same box; only its inset moved right with the wider row.
    assert_px(narrow_detach.size.width, wide_detach.size.width);
    assert!(wide_detach.origin.x > narrow_detach.origin.x);
}
