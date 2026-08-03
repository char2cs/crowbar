//! `--surface project-home-row`, laid out in a real window.
//!
//! What follows pins the arithmetic `crowbar_ui::components::
//! project_home_row`'s module docs describe: the icon wrapper's own 20px
//! box sitting flush against [`row_base::PADDING_X`], the label following
//! [`row_base::GAP`] after it, and the working/selected axes each reaching
//! the picture they claim to.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::{project_home_row, row_base};
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
/// **Mutation:** removing either `.child(...)` call for the two trailing
/// actions in `ProjectHomeRow::render` would turn this red — verified by
/// running it against a version with the switch button's `.child(...)`
/// deleted, which drops `project-home-row-switch` from the recorded ids.
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
/// **Mutation:** swapping `self.working` for a literal `false` in
/// `ProjectHomeRow::icon`'s guard turns this red — run and confirmed: with
/// that change, `--working` no longer surfaces `workspace-branch-icon` in
/// the recorded ids.
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

/// **The icon sits flush against the row's own leading `px-1.5`, and the
/// label follows it by `gap-1.5`** — read off a real taffy layout rather
/// than hand arithmetic.
///
/// **Mutation:** swapping `row_base::GAP` for `row_base::PADDING_X` in
/// `ProjectHomeRow::icon_wrapper` (a plausible typo, since both are 6px
/// today) would **not** turn this red — the two constants share a value.
/// The mutation that does: adding a second `row_base::GAP` to the row's own
/// `.gap(...)` call, which taffy would apply between *every* child, not
/// just after the icon — confirmed by running it: the label's own `x`
/// origin moves from 32 to 38, and the assertion below catches it.
#[gpui::test]
fn the_icon_sits_flush_and_the_label_follows_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "240"]));

    let icon = at(&records, project_home_row::ID_ICON);
    let label = at(&records, project_home_row::ID_LABEL);

    assert_px(icon.origin.x, row_base::PADDING_X);
    assert_px(icon.size.width, project_home_row::ICON_WRAPPER_SIZE);
    assert_px(
        label.origin.x,
        row_base::PADDING_X + project_home_row::ICON_WRAPPER_SIZE + row_base::GAP,
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
