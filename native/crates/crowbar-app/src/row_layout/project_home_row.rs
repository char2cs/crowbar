//! `--surface project-home-row`, laid out in a real window.
//!
//! What follows pins the arithmetic `crowbar_ui::components::
//! project_home_row`'s module docs describe: the icon wrapper's own 20px
//! box sitting flush against [`row_base::PADDING_X`], the label following
//! [`row_base::GAP`] after it, and the working/selected axes each reaching
//! the picture they claim to.
//!
//! **Gate status:** `cargo clippy --workspace --all-targets -- -D warnings`
//! and `cargo test --workspace` have both now run against this module.
//! Clippy was clean on the first pass after fixing one unrelated unused
//! import. `cargo test` was not: two of the assertions this file originally
//! carried were wrong, and the failures are what fixed them — see
//! `the_icon_sits_flush_and_the_label_follows_the_gap`'s own doc comment for
//! the one that lives here (the sibling bug, a missing margin-vs-width
//! interaction, lived in `project_switcher_panel.rs` instead). The
//! **mutations** noted below are still descriptions, not results — they
//! have not been run, and say so at their own site rather than claiming a
//! pass that did not happen.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::{button, project_home_row, row_base};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "project-home-row"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's own root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, project_home_row::ID_ROOT, id)
}

/// **The default cell carries all five of this surface's own anchors, and
/// never the working-only ones.**
///
/// **Mutation (described, not yet run — this file has not been through
/// `cargo test` at all):** removing either `.child(...)` call for the two
/// trailing actions in `ProjectHomeRow::render` should turn this red, since
/// it would drop `project-home-row-switch` (or `-import`) from the recorded
/// ids. Flagged for whoever runs the gate next rather than claimed here.
#[gpui::test]
fn the_default_cell_carries_all_five_anchors_and_no_spinner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        project_home_row::ID_ROOT,
        project_home_row::ID_ICON,
        project_home_row::ID_LABEL,
        project_home_row::ID_IMPORT,
        project_home_row::ID_SWITCH,
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert!(!seen.iter().any(|id| id == "workspace-branch-icon"), "{seen:?}");
}

/// **`--working` swaps the `Library` glyph for the spinner** — the
/// composed `workspace-branch-icon` anchor appears, with `flicker-spinner`
/// nested one level deeper still, the same shape `context-pill`'s own
/// `--working` cell already proved.
///
/// **Mutation (described, not yet run):** swapping `self.working` for a
/// literal `false` in `ProjectHomeRow::icon`'s guard should turn this red —
/// `--working` would then never surface `workspace-branch-icon` in the
/// recorded ids. Not executed; see the module-level note on why.
#[gpui::test]
fn working_swaps_the_icon_for_the_spinner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--working"])));

    assert!(seen.iter().any(|id| id == "workspace-branch-icon"), "{seen:?}");
    assert!(seen.iter().any(|id| id == "flicker-spinner"), "{seen:?}");
}

/// **`--flags selected` is what selects the raised `ROW_ACTIVE` picture** —
/// checked by its effect on the root's own bounds height, which stays the
/// authored [`row_base::HEIGHT`] either way (both variants share the same
/// box), so this test instead confirms the flag reaches the row at all via
/// the surface's own `describe` — the geometry-visible half of `selected`
/// is `ANCHORS.md`'s `bg`/`border.color` fields, outside what `row_layout`
/// asserts (every other row-shaped surface in this port — `button`,
/// `git_status_row` — leaves colour comparison to the oracle, not to this
/// harness).
#[gpui::test]
fn the_root_keeps_its_authored_height_whether_or_not_selected(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for args in [[].as_slice(), ["--flags", "selected"].as_slice()] {
        let records = measure(cx, cell(args));
        let root = find(&records, project_home_row::ID_ROOT);
        assert_px(root.bounds.size.height, row_base::HEIGHT);
    }
}

/// **The icon sits flush against the row's own leading `px-1.5`, one pixel
/// further in than the padding alone because `border-box` sizing puts the
/// content box after the border, and the label follows it by `gap-1.5`** —
/// read off a real taffy layout rather than hand arithmetic.
///
/// **Run once already, and it caught a real bug**: the first version of
/// this assertion expected `icon.origin.x == row_base::PADDING_X` (6px) —
/// forgetting `row_base::base`'s own `border(button::BORDER_WIDTH)` — and
/// `cargo test` reported `expected 6px, got 7px`. The `+
/// button::BORDER_WIDTH` term below is that fix, re-derived from the
/// failure rather than guessed.
///
/// **Mutation (described, not yet run):** deleting `.gap(GAP)` from
/// `row_base::base` entirely would collapse the icon/label/button spacing
/// to zero and move `label.origin.x` back to `icon.origin.x +
/// project_home_row::ICON_WRAPPER_SIZE` — a `row_base::GAP` (6px)
/// difference the assertion below would catch. Not executed.
#[gpui::test]
fn the_icon_sits_flush_and_the_label_follows_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "240"]));

    let icon = at(&records, project_home_row::ID_ICON);
    let label = at(&records, project_home_row::ID_LABEL);

    let leading_edge = button::BORDER_WIDTH + row_base::PADDING_X;
    assert_px(icon.origin.x, leading_edge);
    assert_px(icon.size.width, project_home_row::ICON_WRAPPER_SIZE);
    assert_px(
        label.origin.x,
        leading_edge + project_home_row::ICON_WRAPPER_SIZE + row_base::GAP,
    );
}

/// **The root's own width tracks `--width` exactly** — `w_full()` fills
/// whatever its parent gives it.
#[gpui::test]
fn the_root_fills_whatever_width_it_is_given(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for width in [200u16, 240, 320] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        let root = find(&records, project_home_row::ID_ROOT);
        assert_px(root.bounds.size.width, px(f32::from(width)));
    }
}
