//! `--surface project-switcher-panel`, laid out in a real window.
//!
//! What follows pins the arithmetic `crowbar_ui::surfaces::rows::
//! project_switcher_panel`'s module docs describe: the wrapper's own `py-1`
//! before the first row, each row's own `mx-1.5 my-0.5` inset (the numbers
//! [`super::project_home_row`]'s own module docs argue do *not* apply to a
//! standalone row but *do* apply here, in a real list), the always-present
//! import row, and the `empty` flag's own zero-project picture.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::surfaces::rows::project_switcher_panel;
use crowbar_ui::surfaces::rows::row_base;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "project-switcher-panel"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's own root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, project_switcher_panel::ID_ROOT, id)
}

/// **The default cell (two projects, the first active) carries the root, both
/// project rows and their labels, and the import row and its label — and
/// never a third project row.**
#[gpui::test]
fn the_default_cell_carries_two_rows_and_the_import_row(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        project_switcher_panel::ID_ROOT,
        project_switcher_panel::ID_IMPORT,
        project_switcher_panel::ID_IMPORT_LABEL,
        "project-switcher-panel-row-0",
        "project-switcher-panel-row-0-label",
        "project-switcher-panel-row-1",
        "project-switcher-panel-row-1-label",
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert!(!seen.iter().any(|id| id == "project-switcher-panel-row-2"), "{seen:?}");
}

/// **`--flags empty` carries no project row at all — only the import row.**
///
/// This is the test that actually exercises `StateFlag::Empty`'s real
/// declaration on this surface (`SURFACE.unmodelled` does not list it):
/// [`crate::surfaces::project_switcher_panel::Params::panel`] swaps
/// `ProjectSwitcherPanel::rows` for an empty `Vec` regardless of `--count`.
#[gpui::test]
fn flags_empty_carries_only_the_import_row(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--count", "4", "--flags", "empty"])));

    assert!(seen.contains(&project_switcher_panel::ID_IMPORT.to_owned()), "{seen:?}");
    assert!(
        !seen.iter().any(|id| id.starts_with("project-switcher-panel-row-")),
        "{seen:?}",
    );
}

/// **`--count` drives the row count exactly, past the fixture-name list's
/// own length.**
#[gpui::test]
fn count_drives_the_row_count_exactly(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--count", "4"])));

    for index in 0..4 {
        assert!(
            seen.contains(&format!("project-switcher-panel-row-{index}")),
            "row {index} missing from {seen:?}",
        );
    }
    assert!(!seen.iter().any(|id| id == "project-switcher-panel-row-4"), "{seen:?}");
}

/// **The first row sits inset by the wrapper's own `py-1` plus its own
/// `my-0.5`, and each following row (including the import row) follows by
/// exactly one more [`row_base::HEIGHT`] plus two [`row_base::MARGIN_Y`]s** —
/// read off a real taffy layout rather than the module docs' hand
/// arithmetic.
#[gpui::test]
fn rows_stack_with_the_wrappers_padding_and_each_rows_own_margin(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--count", "2", "--width", "240"]));

    let row0 = at(&records, "project-switcher-panel-row-0");
    let row1 = at(&records, "project-switcher-panel-row-1");
    let import = at(&records, project_switcher_panel::ID_IMPORT);

    assert_px(
        row0.origin.y,
        project_switcher_panel::WRAPPER_PADDING_Y + row_base::MARGIN_Y,
    );
    assert_px(row0.size.height, row_base::HEIGHT);

    let stride = row_base::HEIGHT + row_base::MARGIN_Y * 2.0;
    assert_px(row1.origin.y, row0.origin.y + stride);
    assert_px(import.origin.y, row1.origin.y + stride);
}

/// **Every row (project or import) is inset by the same [`row_base::
/// MARGIN_X`] on both edges** — its own width is the root's own width minus
/// twice that margin.
#[gpui::test]
fn every_row_is_inset_by_the_same_margin_on_both_edges(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--count", "1", "--width", "240"]));

    let root = find(&records, project_switcher_panel::ID_ROOT);
    let row0 = at(&records, "project-switcher-panel-row-0");
    let import = at(&records, project_switcher_panel::ID_IMPORT);

    for row in [row0, import] {
        assert_px(row.origin.x, row_base::MARGIN_X);
        assert_px(
            row.size.width,
            root.bounds.size.width - row_base::MARGIN_X * 2.0,
        );
    }
}

/// **The root's own width tracks `--width` exactly.**
#[gpui::test]
fn the_root_fills_whatever_width_it_is_given(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for width in [200u16, 240, 320] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        let root = find(&records, project_switcher_panel::ID_ROOT);
        assert_px(root.bounds.size.width, px(f32::from(width)));
    }
}

/// **The import row's own label is `font-medium` (500), the same weight as
/// a project row's label beside it** — `ROW_BASE`'s own `text-[13px]
/// font-medium` applies to both row kinds
/// (`crowbar_ui::surfaces::rows::row_base`'s own module docs, and this
/// component's own §1 table in `native/mapping/project-switcher-panel.md`).
///
/// **Mutation, run:** in `ProjectSwitcherPanel::import_row`
/// (`crates/crowbar-ui/src/components/project_switcher_panel.rs`), deleted
/// the `.font_weight(FontWeight::MEDIUM)` call that follows
/// `.font(ui_sans_font(theme))` — reproducing the pre-fix ordering, where
/// `.font(...)` silently overwrote the weight `row_base::label_container`
/// had already set. `cargo test -p crowbar-app --bin crowbar-app
/// row_layout::project_switcher_panel::the_import_labels_weight_matches_the_project_rows`
/// failed as predicted, with the real output:
///
/// ```text
/// thread 'row_layout::project_switcher_panel::the_import_labels_weight_matches_the_project_rows' (277878257) panicked at /private/tmp/crowbar-p364-fixtures/native/crates/crowbar-app/src/row_layout/project_switcher_panel.rs:180:5:
/// import label should be font-medium (500) same as the project row, got 400
/// ```
///
/// Reverted after confirming.
#[gpui::test]
fn the_import_labels_weight_matches_the_project_rows(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--count", "1"]));

    let row_label = find(&records, "project-switcher-panel-row-0-label");
    let row_text = row_label.text.expect("a project row's label paints text");
    assert!(
        (row_text.font.weight - 500.0).abs() < f32::EPSILON,
        "project row should be font-medium (500): {}",
        row_text.font.weight,
    );

    let import_label = find(&records, project_switcher_panel::ID_IMPORT_LABEL);
    let import_text = import_label.text.expect("the import row's label paints text");
    assert!(
        (import_text.font.weight - row_text.font.weight).abs() < f32::EPSILON,
        "import label should be font-medium (500) same as the project row, got {}",
        import_text.font.weight,
    );
}
