//! `--surface workspace-tree`, laid out in a real window.
//!
//! Pins `crowbar_ui::components::workspace_tree`'s module docs: the
//! `project-home-row` family composed unconditionally, the hand-built
//! `scroll-area-root`/`scroll-area-viewport` pair standing in for
//! `ScrollArea::render`, and `--repos` nesting the right number of
//! `repo-section` roots.

use super::{a_cell, assert_px, ids, measure};
use crowbar_driver::Paint;
use crowbar_ui::Theme;
use crowbar_ui::components::{row_base, workspace_tree};
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

/// **The composed `project-home-row` is inset by `row_base::MARGIN_X`/
/// `MARGIN_Y` on both axes** — `crowbar_ui::components::workspace_tree`'s
/// own module docs, and `native/mapping/workspace-tree.md`'s own verdict
/// (cause A): `project-home-row.bounds.{x,y,w}` read `0, 0, 344` against a
/// reference `6, 2, 332`.
///
/// **Mutation, run:** in `WorkspaceTree::render`, swapped
/// `.child(self.home_row(theme, anchors))` back for
/// `.child(self.project_home.render(theme, anchors))` (the pre-fix direct
/// composition, no margin wrapper). `project_home_is_inset_by_row_base_
/// margin_on_both_axes` failed as predicted:
///
/// ```text
/// thread 'row_layout::workspace_tree::project_home_is_inset_by_row_base_margin_on_both_axes' panicked at /private/tmp/crowbar-p366-tree/native/crates/crowbar-app/src/row_layout/workspace_tree.rs:107:5:
/// expected 6px, got 0px
/// ```
///
/// The sibling test below (`the_scroll_area_starts_below_the_home_rows_own_
/// margin`) failed under the identical mutation too:
/// `expected 40px, got 36px`. Reverted after confirming both.
#[gpui::test]
fn project_home_is_inset_by_row_base_margin_on_both_axes(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--repos", "0"]));
    let root = super::find(&records, workspace_tree::ID_ROOT);
    let home = super::find(&records, "project-home-row");

    assert_px(home.bounds.origin.x - root.bounds.origin.x, row_base::MARGIN_X);
    assert_px(home.bounds.origin.y - root.bounds.origin.y, row_base::MARGIN_Y);
    assert_px(
        home.bounds.size.width,
        root.bounds.size.width - row_base::MARGIN_X * 2.0,
    );
}

/// **The scroll area starts `row_base::HEIGHT + row_base::MARGIN_Y * 2`
/// below the tree's own root** — the home row's own height plus its own
/// top *and* bottom margin, both now in the flow.
/// `native/mapping/workspace-tree.md`'s own verdict (cause A):
/// `scroll-area-{root,viewport}.bounds.y` read `36.0` against a reference
/// `40.0`.
#[gpui::test]
fn the_scroll_area_starts_below_the_home_rows_own_margin(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--repos", "0"]));
    let root = super::find(&records, workspace_tree::ID_ROOT);
    let scroll_root = super::find(&records, "scroll-area-root");

    assert_px(
        scroll_root.bounds.origin.y - root.bounds.origin.y,
        row_base::HEIGHT + row_base::MARGIN_Y * 2.0,
    );
}

/// **`--project-name` reaches the composed home row's own label** — before
/// this item, this surface had no way to drive it and
/// `project-home-row-label.text` could only ever read the `'home'`
/// fixture fallback (`native/mapping/workspace-tree.md`'s own verdict,
/// cause C).
///
/// **Mutation, run:** in `Params::tree`
/// (`crowbar-app/src/surfaces/workspace_tree.rs`), replaced the
/// `--project-name`/`--home-active` overrides with a bare
/// `ProjectHomeRow::fixture()` (both flags parsed but never reaching the
/// composed row). Both this test and its sibling below
/// (`home_active_paints_the_composed_row_active`) failed as predicted:
///
/// ```text
/// thread 'row_layout::workspace_tree::project_name_reaches_the_composed_home_rows_label' panicked at /private/tmp/crowbar-p366-tree/native/crates/crowbar-app/src/row_layout/workspace_tree.rs:148:5:
/// assertion `left == right` failed
///   left: "home"
///  right: "oracle-fixture"
/// ```
///
/// Reverted after confirming.
#[gpui::test]
fn project_name_reaches_the_composed_home_rows_label(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--project-name", "oracle-fixture"]));
    let label = super::find(&records, "project-home-row-label");
    let text = label.text.as_ref().expect("the label paints text");

    assert_eq!(text.content.as_ref(), "oracle-fixture");
}

/// **`--home-active` paints the composed home row's own `ROW_ACTIVE`
/// picture** — before this item, this surface had no axis for it and
/// `project-home-row.bg`/`border.color` could only ever read
/// `row_base::inactive`'s resting picture, never the live app's active one
/// (`native/mapping/workspace-tree.md`'s own verdict, cause B).
///
/// **Mutation, run:** the identical mutation `project_name_reaches_the_
/// composed_home_rows_label`'s own doc comment describes. This test
/// failed too, in the same run:
///
/// ```text
/// thread 'row_layout::workspace_tree::home_active_paints_the_composed_row_active' panicked at /private/tmp/crowbar-p366-tree/native/crates/crowbar-app/src/row_layout/workspace_tree.rs:164:5:
/// assertion `left == right` failed
///   left: None
///  right: Solid(Hsla { h: 0.16648702, s: 0.017503321, l: 0.119594134, a: 1.0 })
/// ```
///
/// Reverted after confirming.
#[gpui::test]
fn home_active_paints_the_composed_row_active(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    let inactive = super::find(&measure(cx, cell(&[])), "project-home-row");
    let active = super::find(&measure(cx, cell(&["--home-active"])), "project-home-row");

    assert_eq!(active.background, Paint::Solid(theme.background.value()));
    assert_eq!(active.border_color, Some(theme.background.value()));
    assert_ne!(active.background, inactive.background, "{inactive:?}");
    assert_ne!(active.border_color, inactive.border_color, "{inactive:?}");
}
