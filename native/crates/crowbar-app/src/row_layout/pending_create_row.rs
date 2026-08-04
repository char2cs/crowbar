//! `--surface pending-create-row`, laid out in a real window.
//!
//! Pins the arithmetic `crowbar_ui::surfaces::rows::pending_create_row`'s
//! module docs describe: the icon slot's mutually-exclusive spinner/error
//! pictures, and the outer padding wrapper's own offset reaching the root's
//! absolute origin.

use super::{a_cell, assert_px, find, ids, measure};
use crowbar_ui::surfaces::rows::pending_create_row;
use gpui::{TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "pending-create-row"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// **The default (idle) cell carries the root, the composed spinner icon
/// and the label — never the error-only anchors.**
///
/// **Mutation, run:** changed the second `if self.error` in
/// `PendingCreateRow::render` (the one guarding `.child(self.status(...))
/// .child(self.dismiss(...))`) to `if true`, forcing the status/dismiss
/// pair to append unconditionally while leaving the icon-selection guard
/// untouched. `the_default_cell_is_idle_and_has_no_error_anchors` failed as
/// predicted: `pending-create-row-status should not appear on the idle
/// cell: ["pending-create-row", "workspace-branch-icon", "flicker-spinner",
/// "pending-create-row-label", "pending-create-row-status",
/// "pending-create-row-dismiss"]` — the spinner composition survived (that
/// guard was untouched) while the two error-only anchors leaked onto the
/// idle cell. Reverted after confirming.
#[gpui::test]
fn the_default_cell_is_idle_and_has_no_error_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [pending_create_row::ID_ROOT, "workspace-branch-icon", pending_create_row::ID_LABEL] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    for id in [pending_create_row::ID_ICON, pending_create_row::ID_STATUS, pending_create_row::ID_DISMISS] {
        assert!(!seen.iter().any(|s| s == id), "{id} should not appear on the idle cell: {seen:?}");
    }
}

/// **`--error` swaps the spinner for the row's own `✕` mark, and adds the
/// `"failed"` caption and the dismiss button — the composed
/// `workspace-branch-icon` never appears.**
///
/// **Mutation, run:** replaced `row = row.child(self.status(...))
/// .child(self.dismiss(...))` with `row = row;` in `PendingCreateRow::
/// render`'s error arm, dropping both children while keeping the icon
/// swap. `the_error_cell_swaps_the_icon_and_adds_status_and_dismiss` failed
/// as predicted: `pending-create-row-status missing from ["pending-create-
/// row", "pending-create-row-icon", "pending-create-row-label"]`. Reverted
/// after confirming.
#[gpui::test]
fn the_error_cell_swaps_the_icon_and_adds_status_and_dismiss(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--error"])));

    for id in [
        pending_create_row::ID_ROOT,
        pending_create_row::ID_ICON,
        pending_create_row::ID_LABEL,
        pending_create_row::ID_STATUS,
        pending_create_row::ID_DISMISS,
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert!(!seen.iter().any(|id| id == "workspace-branch-icon"), "{seen:?}");
}

/// **The root's own absolute origin reaches the harness's own horizontal
/// inset plus `--padding-left` plus the row's own `mx-1.5`** — the outer
/// wrapper's own offset, read through to the anchored row inside it, which
/// itself carries [`row_base::MARGIN_X`] (`crowbar_ui::surfaces::rows::
/// pending_create_row`'s own module docs — this row always sits beside
/// siblings, so the margin is real and applied).
///
/// **Mutation, run:** deleted `.pl(self.padding_left)` from
/// `PendingCreateRow::render`'s outer wrapper. `the_root_s_absolute_origin_
/// is_the_inset_plus_padding_left_plus_margin_x` failed as predicted:
/// `expected 44px (24 + 14 + 6), got 30px` (`24 + 6`, `MARGIN_X` alone,
/// with no indentation contributed at all). Reverted after confirming.
#[gpui::test]
fn the_root_s_absolute_origin_is_the_inset_plus_padding_left_plus_margin_x(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--padding-left", "14"]));
    let root = find(&records, pending_create_row::ID_ROOT);

    assert_px(
        root.bounds.origin.x,
        px(crate::row_surface::INSET_X) + px(14.0) + crowbar_ui::surfaces::rows::row_base::MARGIN_X,
    );
}

/// **The root keeps its authored [`row_base::HEIGHT`] whether idle or
/// failed** — the row's own picture varies in content, not in its own
/// box height.
#[gpui::test]
fn the_root_keeps_its_authored_height_idle_or_failed(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for args in [[].as_slice(), ["--error"].as_slice()] {
        let records = measure(cx, cell(args));
        let root = find(&records, pending_create_row::ID_ROOT);
        assert_px(root.bounds.size.height, crowbar_ui::surfaces::rows::row_base::HEIGHT);
    }
}

/// **The label's own line box is `13 × row_base::LINE_HEIGHT_RELATIVE`
/// (19.5px), not the row's authored `h-9`** — `project_home_row.rs`'s own
/// gap-closing test, one label over: a wrong `LINE_HEIGHT_RELATIVE`
/// compiled, passed clippy and passed every other test in this tree before
/// a live parity run caught it (`row_base.rs`'s own doc comment has the
/// full account), because nothing here measured a label's own box, only
/// the row's.
///
/// **Mutation, run:** temporarily set
/// `crowbar_ui::surfaces::rows::row_base::LINE_HEIGHT_RELATIVE` back to its
/// pre-fix value (`18.0 / 13.0`, ≈ `1.3846`) to confirm this test would
/// have caught that exact regression. `cargo test -p crowbar-app --bin
/// crowbar-app the_labels_own_line_box_is_13px_times_the_row_base_ratio`
/// failed as expected: `expected 19.5px, got 18px`. Reverted to `1.5`
/// after confirming.
#[gpui::test]
fn the_labels_own_line_box_is_13px_times_the_row_base_ratio(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let label = find(&records, pending_create_row::ID_LABEL);

    assert_px(
        label.bounds.size.height,
        crowbar_ui::surfaces::rows::row_base::TEXT * crowbar_ui::surfaces::rows::row_base::LINE_HEIGHT_RELATIVE,
    );
    assert_px(label.bounds.size.height, px(19.5));
}
