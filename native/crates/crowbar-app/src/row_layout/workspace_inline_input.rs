//! `--surface workspace-inline-input`: what taffy resolves the rename/create
//! field to, in a real window.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::surfaces::workspace::workspace_inline_input;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, at 248px — the field's own real content width
/// inside the sidebar's `ROW_BASE` row (294px sidebar, less `mx-1.5 px-1.5`
/// twice and the leading icon + gap: `294 − 24 − 22 = 248`, `native/mapping/
/// workspace-inline-input.md`'s own arithmetic).
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "workspace-inline-input", "--width", "248"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to this surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, workspace_inline_input::ID_ROOT, id)
}

/// With no hint, the root carries exactly the field — root at the origin,
/// `248` wide.
#[gpui::test]
fn with_no_hint_the_root_carries_only_the_field(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec![
            "workspace-inline-input".to_owned(),
            "workspace-inline-input-field".to_owned(),
        ],
    );

    let root = at(&records, workspace_inline_input::ID_ROOT);
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(248.0));
}

/// `--hint` adds the third anchor, in source order, after the field.
#[gpui::test]
fn hint_adds_the_third_anchor_in_source_order(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--hint"]));

    assert_eq!(
        ids(&records),
        vec![
            "workspace-inline-input".to_owned(),
            "workspace-inline-input-field".to_owned(),
            "workspace-inline-input-hint".to_owned(),
        ],
    );
}

/// The field's own box: `FIELD_HEIGHT` tall, full root width — box only, no
/// text field at all, `input.rs`'s own finding carried over verbatim.
#[gpui::test]
fn the_field_is_box_only_and_full_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let field = find(&records, workspace_inline_input::ID_FIELD);
    assert!(field.text.is_none(), "the field must carry no text field");
    assert!(!field.content_sized);
    assert!(!field.line_sized);

    let bounds = at(&records, workspace_inline_input::ID_FIELD);
    assert_px(bounds.size.width, px(248.0));
    assert_px(bounds.size.height, workspace_inline_input::FIELD_HEIGHT);
    assert_px(bounds.origin.x, px(0.0));
    assert_px(bounds.origin.y, px(0.0));
}

/// The hint sits `mt-0.5` below the field, is full width (it stretches — the
/// module docs' `text-left`-is-a-tell finding), and paints a real run.
#[gpui::test]
fn the_hint_sits_below_the_field_and_stretches_full_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--hint"]));

    let field = at(&records, workspace_inline_input::ID_FIELD);
    let hint = at(&records, workspace_inline_input::ID_HINT);

    assert_px(
        hint.origin.y,
        field.origin.y + field.size.height + workspace_inline_input::HINT_MARGIN_TOP,
    );
    assert_px(hint.size.width, px(248.0));

    let hint_record = find(&records, workspace_inline_input::ID_HINT);
    assert!(!hint_record.content_sized, "the hint stretches, it does not size to its text");
    assert!(!hint_record.line_sized, "the hint wraps; its box is not a single line box");
    let text = hint_record.text.clone().expect("the hint paints its own run");
    assert_eq!(text.content, "'fix-auth-bug' already has a workspace — open it");
}

/// `--content` moves the same string in the field's own (invisible) value and
/// the hint's real, painted text — the two are the identical value, the
/// module docs' central claim.
#[gpui::test]
fn content_moves_the_same_value_the_hint_names(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let short = measure(cx, cell(&["--hint", "--content", "short"]));
    let overflow = measure(cx, cell(&["--hint", "--content", "overflow"]));

    let short_text = find(&short, workspace_inline_input::ID_HINT)
        .text
        .expect("paints a run");
    let overflow_text = find(&overflow, workspace_inline_input::ID_HINT)
        .text
        .expect("paints a run");
    assert_eq!(short_text.content, "'main' already has a workspace — open it");
    assert!(overflow_text.content.starts_with("'feature/rewrite-the-onboarding-flow"));
    assert!(overflow_text.content.len() > short_text.content.len());
}

/// `empty` drops the field's (invisible) value — the placeholder shows
/// instead — and the hint, when also requested, is derived from the empty
/// string rather than from `--content`. Nothing about the field's own
/// geometry moves: it carries no text field either way.
#[gpui::test]
fn empty_shows_the_placeholder_and_leaves_the_field_box_unchanged(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let filled = measure(cx, cell(&[]));
    let empty = measure(cx, cell(&["--flags", "empty"]));

    let filled_field = at(&filled, workspace_inline_input::ID_FIELD);
    let empty_field = at(&empty, workspace_inline_input::ID_FIELD);
    assert_px(filled_field.size.width, empty_field.size.width);
    assert_px(filled_field.size.height, empty_field.size.height);
    // Same two-anchor shape either way — `empty` does not touch the anchor
    // set, only the invisible value the field would have painted.
    assert_eq!(ids(&filled), ids(&empty));
}

/// `--kind prose` reaches the surface without otherwise moving the field's
/// own box — the two `kind`s share the one `AMBIENT_LINE_HEIGHT`, so the
/// height is unchanged (`workspace_inline_input`'s own module docs).
#[gpui::test]
fn prose_kind_does_not_move_the_field_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let identifier = measure(cx, cell(&["--kind", "identifier"]));
    let prose = measure(cx, cell(&["--kind", "prose"]));

    assert_px(
        at(&identifier, workspace_inline_input::ID_FIELD).size.height,
        at(&prose, workspace_inline_input::ID_FIELD).size.height,
    );
}

/// The surface takes no `--viewport-width` axis: neither anchor carries an
/// `sm:` rule, so the field's height is identical above and below the
/// breakpoint.
#[gpui::test]
fn viewport_width_moves_nothing_on_this_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "1714"]));
    let narrow = measure(cx, cell(&["--viewport-width", "639"]));

    assert_px(
        at(&wide, workspace_inline_input::ID_FIELD).size.height,
        at(&narrow, workspace_inline_input::ID_FIELD).size.height,
    );
}

/// The panel stretches with `--width`, and the hint re-stretches with it too
/// — both track the root rather than each other.
#[gpui::test]
fn the_root_and_hint_stretch_together_with_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(
        cx,
        a_cell(&["--surface", "workspace-inline-input", "--width", "248", "--hint"]),
    );
    let wide = measure(
        cx,
        a_cell(&["--surface", "workspace-inline-input", "--width", "320", "--hint"]),
    );

    assert_px(at(&narrow, workspace_inline_input::ID_ROOT).size.width, px(248.0));
    assert_px(at(&wide, workspace_inline_input::ID_ROOT).size.width, px(320.0));
    assert_px(at(&narrow, workspace_inline_input::ID_HINT).size.width, px(248.0));
    assert_px(at(&wide, workspace_inline_input::ID_HINT).size.width, px(320.0));
}
