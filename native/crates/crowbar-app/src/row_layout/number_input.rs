//! `--surface number-input` — what taffy resolves the root, the two stepper
//! buttons and the field to, measured in a real window against the live
//! app's own numbers.
//!
//! The extractor sees **four** anchors: the root, the decrement button, the
//! field, the increment button. `--width` (the surface column `RowSurface`
//! draws into) is deliberately **not** what moves this surface's picture —
//! the root authors its own width (`--class-width`) and renders at it
//! regardless of the column, the same way a block child with an authored
//! width ignores its container's. Every cell below therefore pins `--width`
//! to a column comfortably wider than any `--class-width` arm (200px against
//! the widest, 144) and drives the picture through `--class-width` instead.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::number_input;
use crowbar_ui::primitives::number_input::{ALL_SIZES, ALL_WIDTHS, Width};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, at the live reference's own geometry: `.number`
/// width (112px), `sm:` breakpoint, `size="xs"`.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "number-input",
        "--width",
        "200",
        "--viewport-width",
        "1714",
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, number_input::ID_ROOT, id)
}

/// Anchor presence is fixed at four on every cell this surface can be driven
/// to — it does not grow or shrink the way `tabs`'s or `dropdown-menu`'s do.
#[gpui::test]
fn every_cell_carries_exactly_the_four_contract_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [
        vec![],
        vec!["--size", "sm"],
        vec!["--class-width", "compact"],
        vec!["--class-width", "default"],
        vec!["--disabled"],
        vec!["--at-min", "--at-max"],
        vec!["--flags", "hover"],
    ] {
        let seen = ids(&measure(cx, cell(&line)));
        assert_eq!(seen.len(), 4, "{line:?}: {seen:?}");
        for id in [
            number_input::ID_ROOT,
            number_input::ID_DECREMENT,
            number_input::ID_FIELD,
            number_input::ID_INCREMENT,
        ] {
            assert!(seen.contains(&id.to_owned()), "{line:?}: {seen:?}");
        }
    }
}

/// **The live reference's own numbers**, reproduced from taffy's layout:
/// root `112×32`, both buttons `32×32`, field `40×24` at `36,4`.
#[gpui::test]
fn the_reference_geometry_matches_the_live_capture(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let root = at(&records, number_input::ID_ROOT);
    let dec = at(&records, number_input::ID_DECREMENT);
    let field = at(&records, number_input::ID_FIELD);
    let inc = at(&records, number_input::ID_INCREMENT);

    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(112.0));
    assert_px(root.size.height, px(32.0));

    assert_px(dec.origin.x, px(0.0));
    assert_px(dec.origin.y, px(0.0));
    assert_px(dec.size.width, px(32.0));
    assert_px(dec.size.height, px(32.0));

    assert_px(field.origin.x, px(36.0));
    assert_px(field.origin.y, px(4.0));
    assert_px(field.size.width, px(40.0));
    assert_px(field.size.height, px(24.0));

    assert_px(inc.origin.x, px(80.0));
    assert_px(inc.origin.y, px(0.0));
    assert_px(inc.size.width, px(32.0));
    assert_px(inc.size.height, px(32.0));

    // The row's own height follows the taller child (the buttons, at `sm:`).
    assert_px(root.size.height, dec.size.height);
}

/// **`min-w-[5ch]` overflows the row at `Width::Compact`** — the module
/// docs' §4 trap, pinned in a real window rather than only in the component's
/// own arithmetic. `96px` authored; the children span `109.25px`.
///
/// The field's own width uses [`assert_within_tolerance`] rather than
/// [`assert_px`]: `MIN_FIELD_WIDTH_SM_TEXT` (37.26px) is a `getComputedStyle`
/// measurement, and gpui pixel-snaps an authored length to the device grid
/// during layout — measured here at **37.5px** in this window's DPR, `0.24px`
/// off the CSS value and comfortably inside `ANCHORS.md` §5's own ±0.5
/// cross-engine tolerance.
#[gpui::test]
fn min_field_width_overflows_the_row_at_compact_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--class-width", "compact"]));

    let root = at(&records, number_input::ID_ROOT);
    let field = at(&records, number_input::ID_FIELD);
    let inc = at(&records, number_input::ID_INCREMENT);

    assert_px(root.size.width, px(96.0));
    assert_within_tolerance(field.size.width, number_input::MIN_FIELD_WIDTH_SM_TEXT);

    let children_span = inc.origin.x + inc.size.width;
    assert!(
        children_span > root.size.width,
        "children span {children_span:?} should overflow the {:?} root",
        root.size.width,
    );

    // `Width::Number` (the live reference) and `Width::Default` do not
    // overflow.
    for width in [Width::Number, Width::Default] {
        let records = measure(cx, cell(&["--class-width", width.name()]));
        let root = at(&records, number_input::ID_ROOT);
        let inc = at(&records, number_input::ID_INCREMENT);
        let span = inc.origin.x + inc.size.width;
        assert!(
            span <= root.size.width,
            "{width:?}: children span {span:?} should not overflow {:?}",
            root.size.width,
        );
    }
}

/// **The buttons carry a 1px transparent border, the field a 1px
/// `theme.border` one, and the root none at all.**
#[gpui::test]
fn each_anchors_border_matches_the_class_list(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_px(find(&records, number_input::ID_ROOT).border_width, px(0.0));
    assert_px(
        find(&records, number_input::ID_DECREMENT).border_width,
        number_input::BUTTON_BORDER_WIDTH,
    );
    assert_px(
        find(&records, number_input::ID_INCREMENT).border_width,
        number_input::BUTTON_BORDER_WIDTH,
    );
    assert_px(
        find(&records, number_input::ID_FIELD).border_width,
        number_input::FIELD_BORDER_WIDTH,
    );

    let field = find(&records, number_input::ID_FIELD);
    assert_eq!(field.border_color, Some(Theme::DARK.border.value()));
}

/// **Radii differ per anchor**: the root has none, the buttons carry
/// `rounded-lg` (10px), the field `rounded-md` (8px).
#[gpui::test]
fn each_anchors_radius_matches_the_class_list(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_px(find(&records, number_input::ID_ROOT).radius, px(0.0));
    assert_px(
        find(&records, number_input::ID_DECREMENT).radius,
        Theme::DARK.radius_lg.value(),
    );
    assert_px(
        find(&records, number_input::ID_INCREMENT).radius,
        Theme::DARK.radius_lg.value(),
    );
    assert_px(
        find(&records, number_input::ID_FIELD).radius,
        Theme::DARK.radius_md.value(),
    );
    assert_ne!(
        find(&records, number_input::ID_DECREMENT).radius,
        find(&records, number_input::ID_FIELD).radius,
    );
}

/// **`hover` paints the buttons' background and nothing else moves.**
#[gpui::test]
fn hover_paints_only_the_two_buttons_background(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&[]));
    let hovered = measure(cx, cell(&["--flags", "hover"]));

    assert_eq!(
        find(&resting, number_input::ID_DECREMENT).background,
        Paint::None
    );
    assert_eq!(
        find(&resting, number_input::ID_INCREMENT).background,
        Paint::None
    );
    assert_eq!(
        find(&hovered, number_input::ID_DECREMENT).background,
        Paint::Solid(Theme::DARK.accent.value()),
    );
    assert_eq!(
        find(&hovered, number_input::ID_INCREMENT).background,
        Paint::Solid(Theme::DARK.accent.value()),
    );

    // The field and the root are untouched.
    assert_eq!(
        find(&resting, number_input::ID_FIELD).background,
        find(&hovered, number_input::ID_FIELD).background,
    );
    for id in [
        number_input::ID_ROOT,
        number_input::ID_DECREMENT,
        number_input::ID_FIELD,
        number_input::ID_INCREMENT,
    ] {
        assert_px(at(&resting, id).size.width, at(&hovered, id).size.width);
    }
}

/// **Only the buttons' height moves with the viewport** — the module docs'
/// §5 finding, measured in a real window.
#[gpui::test]
fn only_the_buttons_height_moves_with_the_viewport(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&[]));
    let narrow = measure(cx, cell(&["--viewport-width", "600"]));

    let wide_dec = at(&wide, number_input::ID_DECREMENT);
    let narrow_dec = at(&narrow, number_input::ID_DECREMENT);
    assert_px(narrow_dec.size.height - wide_dec.size.height, px(4.0));
    assert_px(narrow_dec.size.width, wide_dec.size.width);

    let wide_field = at(&wide, number_input::ID_FIELD);
    let narrow_field = at(&narrow, number_input::ID_FIELD);
    assert_px(narrow_field.size.height, wide_field.size.height);

    // The field re-centres as the row grows taller.
    assert_px(wide_field.origin.y, px(4.0));
    assert_px(narrow_field.origin.y, px(6.0));
}

/// **The field's background and border colours are the bare `muted`/`border`
/// tokens**, not a `.mix()` — pinned as the arithmetic the reference's hex
/// decodes to.
#[gpui::test]
fn the_fields_colours_are_the_bare_tokens(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let field = find(&records, number_input::ID_FIELD);

    assert_eq!(field.background, Paint::Solid(Theme::DARK.muted.value()));
    assert_eq!(field.border_color, Some(Theme::DARK.border.value()));
}

/// **The digit string reaches no anchor**, at any content length — the same
/// finding `input.rs`'s own harness pins, on a second void `<input>`.
#[gpui::test]
fn no_anchor_on_this_surface_reports_text(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [
        vec!["--content", "short"],
        vec!["--content", "normal"],
        vec!["--content", "overflow"],
    ] {
        let records = measure(cx, cell(&line));
        for record in &records {
            assert!(record.text.is_none(), "{line:?}: {} paints text", record.id);
            assert!(!record.content_sized, "{line:?}: {}", record.id);
            assert!(!record.line_sized, "{line:?}: {}", record.id);
        }
    }
}

/// Every size and every width reach the component, in a real window.
#[gpui::test]
fn every_size_and_width_reach_the_component(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for size in ALL_SIZES {
        let records = measure(cx, cell(&["--size", size.name()]));
        assert!(!ids(&records).is_empty(), "{size:?}");
    }
    for width in ALL_WIDTHS {
        let records = measure(cx, cell(&["--class-width", width.name()]));
        let root = at(&records, number_input::ID_ROOT);
        assert_px(root.size.width, width.value());
    }
}

/// Nothing on this surface leaks another surface's anchors.
#[gpui::test]
fn no_cell_of_this_surface_records_a_foreign_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [
        vec!["--size", "sm", "--class-width", "default"],
        vec!["--disabled", "--at-min", "--at-max"],
        vec!["--flags", "hover"],
    ] {
        let seen = ids(&measure(cx, cell(&line)));
        let known = [
            number_input::ID_ROOT,
            number_input::ID_DECREMENT,
            number_input::ID_FIELD,
            number_input::ID_INCREMENT,
        ];
        assert!(
            seen.iter().all(|id| known.contains(&id.as_str())),
            "{line:?}: {seen:?}",
        );
    }
}
