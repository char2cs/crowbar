//! `--surface context-pill`, laid out in a real window.
//!
//! What follows pins the arithmetic `crowbar_ui::components::context_pill`'s
//! module docs describe: the two-line label stack's own gap and line
//! heights, the trailing glyph's own precedence (avatar beats status icon,
//! spinner beats avatar), and the wrapper's own outer padding.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::context_pill;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "context-pill"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's own root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, context_pill::ID_ROOT, id)
}

/// **The default cell carries exactly the root, the trigger, and the
/// composed status glyph's own anchor — never `repo-avatar`, since the
/// default cell has no avatar.**
///
/// **Mutation:** deleting the `!working` half of `context_pill.rs`'s
/// `workspace_glyph`'s guard (leaving only `repo_avatar.is_some()`) does not
/// redden this test, because the default cell's own `repo_avatar` is
/// already `None` — the case that catches it is
/// `the_spinner_beats_the_avatar_when_working` below.
#[gpui::test]
fn the_default_cell_carries_the_root_the_trigger_and_the_status_glyph(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        context_pill::ID_ROOT,
        context_pill::ID_TRIGGER,
        "workspace-branch-icon",
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert!(!seen.iter().any(|id| id == "repo-avatar"), "{seen:?}");
}

/// **`--avatar` swaps the status glyph for `repo-avatar`, and `--working`
/// swaps it right back** — the two-step precedence
/// `context_pill.rs`'s own `workspace_glyph` documents, each half exercised
/// on the axis the other cannot reach alone.
///
/// **Mutation:** swapping the `Some(avatar) if !working` guard for a bare
/// `Some(avatar)` in `ContextPill::workspace_glyph` turns the second
/// assertion red — a working, avatar-bearing cell would then show
/// `repo-avatar`, not `workspace-branch-icon`.
#[gpui::test]
fn the_spinner_beats_the_avatar_when_working(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let with_avatar = ids(&measure(cx, cell(&["--avatar"])));
    assert!(with_avatar.iter().any(|id| id == "repo-avatar"), "{with_avatar:?}");
    assert!(
        !with_avatar.iter().any(|id| id == "workspace-branch-icon"),
        "{with_avatar:?}",
    );

    let working_with_avatar = ids(&measure(cx, cell(&["--avatar", "--working"])));
    assert!(
        working_with_avatar.iter().any(|id| id == "workspace-branch-icon"),
        "{working_with_avatar:?}",
    );
    assert!(
        !working_with_avatar.iter().any(|id| id == "repo-avatar"),
        "{working_with_avatar:?}",
    );
    // And the spinner's own nested anchor, one level deeper still —
    // `WorkspaceBranchIcon::render`'s `working` branch.
    assert!(working_with_avatar.iter().any(|id| id == "flicker-spinner"));
}

/// **`--kind home`/`--kind project` each carry the root and the trigger, and
/// neither ever carries `repo-avatar`** — only a `--kind workspace` cell has
/// a repo avatar to show at all.
#[gpui::test]
fn home_and_project_kinds_carry_no_repo_avatar(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for kind in ["home", "project"] {
        let seen = ids(&measure(cx, cell(&["--kind", kind])));
        assert!(seen.contains(&context_pill::ID_ROOT.to_owned()), "{kind}: {seen:?}");
        assert!(seen.contains(&context_pill::ID_TRIGGER.to_owned()), "{kind}: {seen:?}");
        assert!(!seen.iter().any(|id| id == "repo-avatar"), "{kind}: {seen:?}");
    }
}

/// **The trigger sits inset by the wrapper's own `px-2` on both edges, and
/// its own top edge is the wrapper's own top** (`pt-0`) — read off a real
/// taffy layout rather than the module docs' hand arithmetic.
///
/// **Mutation:** swapping `OUTER_PADDING_X` for `OUTER_PADDING_BOTTOM`'s own
/// value (4px) in `ContextPill::wrapper` turns the width assertion below
/// red — the trigger would then sit 4px narrower than the root on each
/// side, not 8.
#[gpui::test]
fn the_trigger_sits_inset_by_the_wrappers_own_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "240"]));

    let root = find(&records, context_pill::ID_ROOT);
    let trigger = at(&records, context_pill::ID_TRIGGER);

    assert_px(trigger.origin.x, context_pill::OUTER_PADDING_X);
    assert_px(trigger.origin.y, px(0.0)); // pt-0
    assert_px(
        trigger.size.width,
        root.bounds.size.width - context_pill::OUTER_PADDING_X * 2.0,
    );
}

/// **The root's own width tracks `--width` exactly** — `w_full()` fills
/// whatever its parent gives it, not an authored length.
#[gpui::test]
fn the_root_fills_whatever_width_it_is_given(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for width in [200u16, 240, 320] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        let root = find(&records, context_pill::ID_ROOT);
        assert_px(root.bounds.size.width, px(f32::from(width)));
    }
}

/// **The live parity cell this surface once failed on**
/// (`--kind home --width 344`, a 1714px viewport, dark) — the trigger's own
/// border-box height is `47px` and it carries a real, transparent 1px
/// border, both confirmed against the live app's own `getComputedStyle`.
///
/// This is the regression the mismatched `LEADING_TIGHT` constant (once a
/// borrowed, wrong `px(18.0)`) and the missing `button::BORDER_WIDTH` both
/// produced together — see [`context_pill::LEADING_TIGHT`]'s own doc for
/// the arithmetic. `assert_within_tolerance` is `ANCHORS.md` §5's own ±0.5,
/// the same slack a sub-pixel line-height computation is allowed; the
/// border width is asserted exactly, because `ANCHORS.md` compares
/// `border.w` exactly regardless of tolerance elsewhere.
///
/// **Mutation:** reverting `LEADING_TIGHT` to the old borrowed `px(18.0)`
/// (as a fixed line height rather than the `relative(1.25)` ratio) turns
/// the height assertion red again — 48px, a whole pixel outside the ±0.5
/// window.
#[gpui::test]
fn the_live_parity_cell_matches_the_reference_within_tolerance(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--kind", "home", "--width", "344"]));

    let trigger = find(&records, context_pill::ID_TRIGGER);
    assert_within_tolerance(trigger.bounds.size.height, px(47.0));
    assert_px(trigger.border_width, px(1.0));
    assert_eq!(
        trigger.border_color.map(|c| c.a),
        Some(0.0),
        "the border is real width, transparent colour — {:?}",
        trigger.border_color,
    );

    let root = find(&records, context_pill::ID_ROOT);
    assert_within_tolerance(root.bounds.size.height, px(51.0));
}
