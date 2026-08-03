//! `--surface workspace-tree-item`, laid out in a real window.
//!
//! Pins the arithmetic `crowbar_ui::components::workspace_tree_item`'s
//! module docs describe: the leaf/parent trailing-action alternation, the
//! `depth * 14` indentation between a row and its own child, and the
//! `mx-1.5`-net-of-margin width every row in this cluster shares — the
//! `.w_full()` trap the item brief calls out by name.

use super::{a_cell, assert_px, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::{row_base, workspace_tree_item};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "workspace-tree-item"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// The `n`th (0-indexed) record carrying `id`, in paint order — this
/// surface's own root/icon ids repeat once per row when children are
/// present (`workspace_tree_item.rs`'s own module docs), so `super::find`'s
/// first-match lookup cannot tell a parent's own anchor from a child's.
#[track_caller]
fn nth(records: &[RawAnchor], id: &str, n: usize) -> RawAnchor {
    records
        .iter()
        .filter(|record| record.id == id)
        .nth(n)
        .unwrap_or_else(|| panic!("{id} #{n} was not recorded; got {:?}", ids(records)))
        .clone()
}

/// **The default (leaf) cell carries the root, the composed icon, the
/// label and the add-child action — never the expand chevron.**
///
/// **Mutation, run:** swapped `self.has_children()` for `true` in
/// `WorkspaceTreeItem::trailing_button`'s guard.
/// `the_default_leaf_cell_shows_add_child_not_expand` failed as predicted:
/// `workspace-tree-item-add-child missing from ["workspace-tree-item",
/// "workspace-branch-icon", "workspace-tree-item-label",
/// "workspace-tree-item-expand"]` — the chevron appeared on a childless
/// cell instead of the add-child action. Reverted after confirming.
#[gpui::test]
fn the_default_leaf_cell_shows_add_child_not_expand(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        workspace_tree_item::ID_ROOT,
        "workspace-branch-icon",
        workspace_tree_item::ID_LABEL,
        workspace_tree_item::ID_ADD_CHILD,
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert!(!seen.iter().any(|id| id == workspace_tree_item::ID_EXPAND), "{seen:?}");
}

/// **`--children 1` swaps the leaf's add-child action for the expand
/// chevron, and recurses: a second `workspace-tree-item` root (the child's
/// own) is recorded.**
///
/// **Mutation, run:** deleted the `for child in &self.children { ... }`
/// loop body in `WorkspaceTreeItem::children_section`, keeping the
/// `show_children_section` guard (so the section still opens) but never
/// pushing a child. `children_1_recurses_into_a_second_root_anchor` failed
/// as predicted: only one `workspace-tree-item` root was recorded, not two.
/// Reverted after confirming.
#[gpui::test]
fn children_1_recurses_into_a_second_root_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--children", "1"]));
    let seen = ids(&records);

    // The parent (has a child) shows the expand chevron; the child itself
    // (a leaf, `WorkspaceTreeItem::fixture()`-shaped) legitimately shows
    // its own add-child action — both ids coexist in this one capture.
    assert!(seen.contains(&workspace_tree_item::ID_EXPAND.to_owned()), "{seen:?}");
    assert!(seen.contains(&workspace_tree_item::ID_ADD_CHILD.to_owned()), "{seen:?}");
    assert_eq!(
        records.iter().filter(|r| r.id == workspace_tree_item::ID_ROOT).count(),
        2,
        "{seen:?}",
    );
}

/// **A child sits exactly one indent level — `depth * 14` — to the right of
/// its parent**, `native/mapping/layout-denominator.md`'s own denominator
/// arithmetic for this cluster.
///
/// **Mutation, run:** in `crowbar-app/src/surfaces/workspace_tree_item.rs`'s
/// `Params::row`, changed the generated children's own `depth: u32::from
/// (self.depth) + 1` to `depth: u32::from(self.depth)` (no increment).
/// `a_child_sits_one_indent_level_right_of_its_parent` failed as predicted:
/// `expected 14px, got 0px` — the child's own row landed flush with its
/// parent's, at the same depth. Reverted after confirming. (An earlier
/// attempt mutating `WorkspaceTreeItem::depth_padding`'s own multiplier —
/// `levels as f32` to `(levels as f32) - 1.0` — passed unmutated: an
/// additive shift cancels in a *delta* between two calls at consecutive
/// `levels`, so that mutation was not this assertion's own — recorded here
/// rather than discarded, since a mutation that does not bite is itself a
/// finding about which line actually carries the assertion's weight.)
#[gpui::test]
fn a_child_sits_one_indent_level_right_of_its_parent(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--children", "1"]));

    let parent_root = nth(&records, workspace_tree_item::ID_ROOT, 0);
    let child_root = nth(&records, workspace_tree_item::ID_ROOT, 1);

    assert_px(
        child_root.bounds.origin.x - parent_root.bounds.origin.x,
        workspace_tree_item::INDENT,
    );
}

/// **`--pending 1` nests a `pending-create-row` (and its own composed
/// `workspace-branch-icon`) under the children section, at `(depth + 2) *
/// 14`.**
///
/// **Mutation, run:** deleted the `for pending in &self.pending_creates {
/// ... }` loop in `WorkspaceTreeItem::children_section`.
/// `pending_1_nests_a_pending_create_row` failed as predicted:
/// `pending-create-row` was absent from the recorded ids even though
/// `--pending 1` was passed. Reverted after confirming.
#[gpui::test]
fn pending_1_nests_a_pending_create_row(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--pending", "1"]));
    let seen = ids(&records);

    assert!(seen.contains(&"pending-create-row".to_owned()), "{seen:?}");

    let pending_root = super::find(&records, "pending-create-row");
    let row_root = super::find(&records, workspace_tree_item::ID_ROOT);
    // `(0 + 2) * 14` — one level deeper than this row's own `(0 + 1) * 14`.
    assert_px(pending_root.bounds.origin.x - row_root.bounds.origin.x, px(14.0));
}

/// **The row is inset by `MARGIN_X` on both edges (net of its own
/// `padding-left` wrapper) — never `width: 100%` unconditionally
/// overflowing it.** The exact bug the item brief calls out by name:
/// `.w_full()` next to `.mx()` does not shrink for its own margin the way
/// flex stretch does.
///
/// **Mutation, run:** added `.w_full()` to the row shell in
/// `WorkspaceTreeItem::row`, alongside its existing `.mx(row_base::
/// MARGIN_X)`. `the_row_is_inset_by_margin_x_on_both_edges` failed as
/// predicted: at `--width 240` and the default `--row-depth 0` (`row_padding_
/// left` `14px`), the root's own width came back `226px`
/// (`240 - 14`, `row_base::MARGIN_X` not subtracted at all) instead of the
/// expected `214px` (`240 - 14 - 2 * MARGIN_X`) — the same overflow
/// `project_switcher_panel.rs`'s own comment documents having been caught
/// by this exact shape of assertion. Reverted after confirming.
#[gpui::test]
fn the_row_is_inset_by_margin_x_on_both_edges(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "240"]));
    let root = super::find(&records, workspace_tree_item::ID_ROOT);

    // `--row-depth 0` (the default): `row_padding_left` is `(0 + 1) * 14`.
    let available = px(240.0) - workspace_tree_item::INDENT;
    assert_px(root.bounds.size.width, available - row_base::MARGIN_X * 2.0);
}

/// **The root keeps its authored [`row_base::HEIGHT`] whether or not
/// selected** — colour is the only field `selected` moves, outside what
/// this harness asserts (every other row-shaped surface in this port takes
/// the same posture).
#[gpui::test]
fn the_root_keeps_its_authored_height_whether_or_not_selected(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for args in [[].as_slice(), ["--flags", "selected"].as_slice()] {
        let records = measure(cx, cell(args));
        let root = super::find(&records, workspace_tree_item::ID_ROOT);
        assert_px(root.bounds.size.height, row_base::HEIGHT);
    }
}

/// Bounds relative to this surface's own root — used where a single-row
/// (non-recursing) cell makes `super::relative_to`'s first-match lookup
/// unambiguous.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, workspace_tree_item::ID_ROOT, id)
}

/// **The icon sits flush against the row's own leading `px-1.5` (plus the
/// row's own border), and the label follows by `gap-1.5`** — read off a
/// real taffy layout, `project_home_row.rs`'s own assertion shape.
#[gpui::test]
fn the_icon_sits_flush_and_the_label_follows_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "240"]));

    let icon = at(&records, "workspace-branch-icon");
    let label = at(&records, workspace_tree_item::ID_LABEL);

    let leading_edge = crowbar_ui::components::button::BORDER_WIDTH + row_base::PADDING_X;
    assert_px(icon.origin.x, leading_edge);
    assert_px(label.origin.x, leading_edge + px(16.0) + row_base::GAP);
}
