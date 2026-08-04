//! `--surface workspace-tree`, laid out in a real window.
//!
//! Pins `crowbar_ui::components::workspace_tree`'s module docs: the
//! `project-home-row` family composed unconditionally, the hand-built
//! `scroll-area-root`/`scroll-area-viewport` pair standing in for
//! `ScrollArea::render`, and `--repos` nesting the right number of
//! `repo-section` roots.

use super::{a_cell, ids, measure};
use crowbar_ui::components::workspace_tree;
use gpui::TestAppContext;

use crate::row_surface::Cell;

/// A cell on this surface.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "workspace-tree"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// **The default cell (one repo) carries the root, the full
/// `project-home-row` family, the hand-built scroll-area pair, and one
/// nested `repo-section`.**
///
/// **Mutation, run:** deleted `.child(self.scroll_area(theme, anchors))`
/// from `WorkspaceTree::render`, leaving only the project-home row.
/// `the_default_cell_carries_the_scaffold_and_one_repo_section` failed as
/// predicted: `scroll-area-root missing from ["workspace-tree",
/// "project-home-row", "project-home-row-icon", "project-home-row-label",
/// "project-home-row-import", "project-home-row-switch"]` —
/// `scroll-area-viewport` and `repo-section` were absent too. Reverted
/// after confirming.
#[gpui::test]
fn the_default_cell_carries_the_scaffold_and_one_repo_section(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        workspace_tree::ID_ROOT,
        "project-home-row",
        "project-home-row-icon",
        "project-home-row-label",
        "project-home-row-import",
        "project-home-row-switch",
        "scroll-area-root",
        "scroll-area-viewport",
        "repo-section",
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
}

/// **`--repos 3` nests three `repo-section` roots.**
///
/// **Mutation, run:** changed `(0..self.repos)` to `(0..1)` in
/// `Params::tree` (`crowbar-app/src/surfaces/workspace_tree.rs`).
/// `repos_3_nests_three_repo_section_roots` failed as predicted: `left: 1,
/// right: 3` — three was requested and one was recorded. Reverted after
/// confirming.
#[gpui::test]
fn repos_3_nests_three_repo_section_roots(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--repos", "3"]));
    assert_eq!(records.iter().filter(|r| r.id == "repo-section").count(), 3, "{:?}", ids(&records));
}

/// **`--repos 0` still carries the scaffold — no `repo-section` at all,
/// nothing else missing.**
#[gpui::test]
fn repos_0_carries_the_scaffold_with_no_repo_sections(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--repos", "0"])));

    assert!(seen.contains(&workspace_tree::ID_ROOT.to_owned()), "{seen:?}");
    assert!(seen.contains(&"project-home-row".to_owned()), "{seen:?}");
    assert!(!seen.iter().any(|id| id == "repo-section"), "{seen:?}");
}
