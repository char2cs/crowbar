//! `--surface repo-section`, laid out in a real window.
//!
//! Pins the arithmetic `crowbar_ui::components::repo_section`'s module docs
//! describe: the header's own trailing-action set, `--collapsed` hiding the
//! workspace list entirely, and the header row's own `mx-1.5`-net-of-margin
//! width.

use super::{a_cell, assert_px, ids, measure};
use crowbar_ui::components::{repo_section, row_base};
use gpui::{TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "repo-section"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// **The default cell (one expanded root, a default workspace) carries the
/// header's full anchor set plus one nested `workspace-tree-item`.**
///
/// **Mutation, run:** changed `if self.has_default_workspace` to `if
/// false` in `RepoSection::header`, so the add-child button never appends.
/// `the_default_cell_carries_the_full_header_and_one_root` failed as
/// predicted: `repo-section-add-child missing from ["repo-section",
/// "repo-icon-popover-trigger", "repo-section-label", "repo-section-
/// import", "repo-section-collapse", "workspace-tree-item", "workspace-
/// branch-icon", "workspace-tree-item-label", "workspace-tree-item-add-
/// child"]` — the button vanished even with `has_default_workspace`
/// defaulting to `true`. Reverted after confirming.
#[gpui::test]
fn the_default_cell_carries_the_full_header_and_one_root(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        repo_section::ID_ROOT,
        "repo-icon-popover-trigger",
        repo_section::ID_LABEL,
        repo_section::ID_IMPORT,
        repo_section::ID_ADD_CHILD,
        repo_section::ID_COLLAPSE,
        "workspace-tree-item",
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
}

/// **`--no-default-workspace` drops the add-child action.**
#[gpui::test]
fn no_default_workspace_drops_the_add_child_action(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--no-default-workspace"])));
    assert!(!seen.iter().any(|id| id == repo_section::ID_ADD_CHILD), "{seen:?}");
}

/// **`--collapsed` hides the workspace list entirely — no
/// `workspace-tree-item` reaches the capture — while the header stays.**
///
/// **Mutation, run:** changed `if !self.is_collapsed` to `if true` in
/// `RepoSection::render`. `collapsed_hides_the_workspace_list_entirely`
/// failed as predicted: `workspace-tree-item` (and its own nested
/// `workspace-branch-icon`/`-label`/`-add-child`) were present in a
/// `--collapsed` capture. Reverted after confirming.
#[gpui::test]
fn collapsed_hides_the_workspace_list_entirely(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--collapsed"])));

    assert!(seen.contains(&repo_section::ID_ROOT.to_owned()), "{seen:?}");
    assert!(!seen.iter().any(|id| id == "workspace-tree-item"), "{seen:?}");
}

/// **`--roots 3` nests three `workspace-tree-item` roots.**
#[gpui::test]
fn roots_3_nests_three_workspace_tree_item_roots(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--roots", "3"]));
    assert_eq!(records.iter().filter(|r| r.id == "workspace-tree-item").count(), 3, "{:?}", ids(&records));
}

/// **The header row is inset by `MARGIN_X` on both edges** — the same
/// `.w_full()`-next-to-`.mx()` trap `workspace_tree_item.rs`'s own test
/// guards.
///
/// **Mutation, run:** added `.w_full()` to the header shell in
/// `RepoSection::header`, alongside its existing `.mx(row_base::MARGIN_X)`.
/// `the_header_is_inset_by_margin_x_on_both_edges` failed as predicted: the
/// root's own width at `--width 240` came back `240px` instead of the
/// expected `228px`. Reverted after confirming.
#[gpui::test]
fn the_header_is_inset_by_margin_x_on_both_edges(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "240"]));
    let root = super::find(&records, repo_section::ID_ROOT);

    assert_px(root.bounds.size.width, px(240.0) - row_base::MARGIN_X * 2.0);
}

/// **The header keeps its authored [`row_base::HEIGHT`] whether or not
/// selected.**
#[gpui::test]
fn the_header_keeps_its_authored_height_whether_or_not_selected(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for args in [[].as_slice(), ["--flags", "selected"].as_slice()] {
        let records = measure(cx, cell(args));
        let root = super::find(&records, repo_section::ID_ROOT);
        assert_px(root.bounds.size.height, row_base::HEIGHT);
    }
}
