//! `--surface workspace-branch-icon`: what taffy resolves the sidebar's
//! status glyph to, in a real window.
//!
//! Every glyph box is **geometrically identical regardless of status** — same
//! `16 × 16`, same `radius 0`, same `border 0`, same `bg None` — because the
//! six colours this component paints have no field in the contract at all
//! (see the component's own module docs): a box that sets `text_color` but
//! paints no text run emits no `fg`, `flicker-spinner`'s own finding. So the
//! geometry tests below assert the box shape, and the colour table is
//! verified separately, in `crowbar_ui::components::workspace_branch_icon`'s
//! own `#[cfg(test)]` module, by reading `WorkspaceBranchIcon::color`
//! directly — the only place the six colours are comparable to anything.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::workspace_branch_icon::{self, ALL_STATUSES};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "workspace-branch-icon"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the icon itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, workspace_branch_icon::ID, id)
}

/// **The default cell is the idle `new` branch glyph**: `16 × 16`, `radius
/// 0`, `border.w 0`, no background, no text — an empty box, the same
/// `avatar.rs`/`git_status_row` call for an SVG neither engine can draw.
///
/// The one-line mutation: add `.bg(color)` to
/// `WorkspaceBranchIcon::glyph_box` — `record.background` would then read
/// `Paint::Solid(_)` against this test's `Paint::None`.
#[gpui::test]
fn the_default_cell_is_an_empty_16px_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["workspace-branch-icon".to_owned()]);

    let root = at(&records, "workspace-branch-icon");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let record = find(&records, "workspace-branch-icon");
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert_eq!(record.background, Paint::None);
    assert!(
        record.text.is_none(),
        "no text run means no field records the glyph's colour either",
    );
}

/// **Every status resolves to the same box shape.** Seven statuses, one
/// picture geometrically — the colour is the only thing that would move, and
/// it has no field. Read together with `deleted_and_pr_closed_are_the_same_
/// picture` in the component's own tests, which confirms two of the seven
/// share even that.
#[gpui::test]
fn every_status_is_the_same_16px_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    assert_eq!(ALL_STATUSES.len(), 7);

    for status in ALL_STATUSES {
        let records = measure(cx, cell(&["--status", status.name()]));
        let record = find(&records, "workspace-branch-icon");
        assert_px(record.bounds.size.width, px(16.0));
        assert_px(record.bounds.size.height, px(16.0));
        assert_px(record.radius, px(0.0));
        assert_px(record.border_width, px(0.0));
        assert_eq!(record.background, Paint::None, "{}", status.name());
    }
}

/// **`--placeholder` is the same box as every status** — `isPlaceholder`'s
/// `Warning` and `pr-conflicts`'s `Warning` are pixel-identical, and the
/// component's own tests already prove they are the *same colour*; this
/// confirms the box around it is the same shape too.
#[gpui::test]
fn placeholder_is_the_same_box_shape_as_a_status(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let placeholder = measure(cx, cell(&["--placeholder"]));
    let status = measure(cx, cell(&["--status", "locked"]));

    let a = find(&placeholder, "workspace-branch-icon");
    let b = find(&status, "workspace-branch-icon");
    assert_eq!(a.bounds.size, b.bounds.size);
    assert_eq!(a.radius, b.radius);
    assert_eq!(a.background, b.background);
}

/// **`--working` is a different picture entirely: two anchors, not one.**
/// The `16 × 16` wrapper and, centred inside it, `flicker-spinner`'s own
/// `size-3.5` (14px) box — `WorkspaceAgentSpinner`'s exact structure.
///
/// The one-line mutation: change `SpinnerCallSite::WorkspaceIcon` to
/// `SpinnerCallSite::ChatPane` in `WorkspaceBranchIcon::render` — the nested
/// box would then measure `24 × 24` against this test's `14 × 14`, and it
/// would no longer be centred in a `16 × 16` parent.
#[gpui::test]
fn working_draws_the_wrapper_and_the_centred_flicker_spinner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--working"]));

    assert_eq!(
        ids(&records),
        vec!["workspace-branch-icon".to_owned(), "flicker-spinner".to_owned()],
    );

    let wrapper = at(&records, "workspace-branch-icon");
    assert_px(wrapper.size.width, px(16.0));
    assert_px(wrapper.size.height, px(16.0));

    let spinner = at(&records, "flicker-spinner");
    assert_px(spinner.size.width, px(14.0));
    assert_px(spinner.size.height, px(14.0));
    // Centred in the 16px wrapper: (16 - 14) / 2 = 1px on each side.
    assert_px(spinner.origin.x, px(1.0));
    assert_px(spinner.origin.y, px(1.0));
}

/// `--working` beats `--placeholder` beats `--status` — read off the
/// snapshot's own anchor *set*, not just the component's `glyph()` method:
/// only a working cell carries two anchors.
///
/// The one-line mutation: swap the `if self.working` / `else` branches in
/// `WorkspaceBranchIcon::render` — this test's working cell would then
/// record one anchor and its glyph cell two.
#[gpui::test]
fn only_a_working_cell_carries_two_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let working = measure(cx, cell(&["--status", "locked", "--placeholder", "--working"]));
    assert_eq!(ids(&working).len(), 2, "the spinner cell");

    let placeholder = measure(cx, cell(&["--status", "locked", "--placeholder"]));
    assert_eq!(ids(&placeholder).len(), 1, "a plain glyph cell");

    let status = measure(cx, cell(&["--status", "pr-conflicts"]));
    assert_eq!(ids(&status).len(), 1, "a plain glyph cell");
}

/// **`--flags empty` is inert on this surface, glyph cell and spinner cell
/// alike** — `StateFlag::Empty` is unmodelled here (this surface declares
/// `no_state_axis`, `surface.rs`'s own module docs), so driving it has to
/// render the resting picture on both sides rather than collapsing anything.
///
/// The one-line mutation: reintroducing an `empty: cell.has(StateFlag::Empty)`
/// read in `Params::icon` (removed in the P3.50 follow-up that deleted
/// `WorkspaceBranchIcon::empty`) would make the flagged glyph cell measure
/// `0 × 0` against this test's `16 × 16`.
#[gpui::test]
fn empty_is_inert_on_both_the_glyph_and_the_spinner_cells(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let glyph = measure(cx, cell(&["--flags", "empty"]));
    let root = at(&glyph, "workspace-branch-icon");
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let spinner = measure(cx, cell(&["--working", "--flags", "empty"]));
    let wrapper = at(&spinner, "workspace-branch-icon");
    assert_px(wrapper.size.width, px(16.0));
    assert_px(wrapper.size.height, px(16.0));
    let nested = at(&spinner, "flicker-spinner");
    assert_px(nested.size.width, px(14.0));
    assert_px(nested.size.height, px(14.0));
}
