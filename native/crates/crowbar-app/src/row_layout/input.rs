//! `--surface input` — what taffy resolves the control and the field to,
//! measured in a real window against the live app's own numbers.
//!
//! The extractor sees **two** anchors on this surface and no more. Everything
//! else `input.tsx` paints is unanchorable on the *reference* side: the
//! `::before` overlay is a pseudo-element, the `leftIcon` is a component a call
//! site passes, and the value, the placeholder, the caret and the selection have
//! either no node or no field. Anchoring any of it here would put a record in
//! the snapshot the DOM extractor can never produce.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::input;
use crowbar_ui::primitives::input::{ALL_SIZES, Size};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, at the live reference's own geometry: the tree
/// filter's 246px control inside a window much wider than it.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "input",
        "--width",
        "246",
        "--viewport-width",
        "1714",
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, input::ID_CONTROL, id)
}

/// Anchor presence is what the differ ranks first, and this surface's set is
/// **fixed at two** — it does not move with the cell the way `tabs`'s and
/// `button`'s do.
#[gpui::test]
fn every_cell_carries_exactly_the_two_contract_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [
        vec![],
        vec!["--value"],
        vec!["--icon"],
        vec!["--size", "lg", "--class-ps", "none"],
        vec!["--disabled", "--invalid"],
        vec!["--flags", "focus,empty"],
    ] {
        let seen = ids(&measure(cx, cell(&line)));
        assert_eq!(seen.len(), 2, "{line:?}: {seen:?}");
        assert!(seen.contains(&input::ID_CONTROL.to_owned()), "{seen:?}");
        assert!(seen.contains(&input::ID_FIELD.to_owned()), "{seen:?}");
    }
}

/// **The live reference's own numbers**, reproduced from taffy's layout.
///
/// Measured on the running app at `innerWidth` 1714: the control at
/// `246×28`, and the field at `21,1` measuring `224×26` inside it. The two
/// insets are the whole geometry of this surface — 21 is `ps-5`'s 20 plus the
/// border pixel, and 224 is 246 less the two border pixels and the 20.
#[gpui::test]
fn the_control_and_the_field_are_the_live_references_boxes(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = at(&records, input::ID_CONTROL);
    let field = at(&records, input::ID_FIELD);

    assert_px(control.origin.x, px(0.0));
    assert_px(control.origin.y, px(0.0));
    assert_px(control.size.width, px(246.0));
    assert_px(control.size.height, px(28.0));

    assert_px(field.origin.x, px(21.0));
    assert_px(field.origin.y, px(1.0));
    assert_px(field.size.width, px(224.0));
    assert_px(field.size.height, px(26.0));

    // The control's height is the field's plus its own two border pixels —
    // it authors no height of its own at all.
    assert_px(
        control.size.height,
        field.size.height + input::BORDER_WIDTH * 2.0,
    );
    // And the field ends flush with the control's padding box on the right,
    // which is the property a port that mis-resolved `w-full` would miss.
    assert_px(
        field.origin.x + field.size.width + input::BORDER_WIDTH,
        control.size.width,
    );
}

/// **The control carries a real 1px border and the field carries none.**
///
/// Both traps `native/MAPPING.md` records, on one component and on two
/// different elements. `border.w` is the field `ANCHORS.md` v1.1 compares
/// *exactly*, so getting either wrong is a delta on every cell.
#[gpui::test]
fn only_the_control_has_a_border_and_focus_does_not_widen_it(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [vec![], vec!["--flags", "focus"], vec!["--invalid"]] {
        let records = measure(cx, cell(&line));
        assert_px(
            find(&records, input::ID_CONTROL).border_width,
            input::BORDER_WIDTH,
        );
        assert_px(find(&records, input::ID_FIELD).border_width, px(0.0));
    }

    // `has-focus-visible:ring-[3px]` is a **box-shadow**, so it is three
    // times the border and reaches the border not at all.
    assert_px(input::RING_SPREAD, px(3.0));
    assert!(input::RING_SPREAD > input::BORDER_WIDTH);
}

/// **`rounded-[inherit]` gives the field the control's radius**, and the
/// control paints the background while the field paints none.
#[gpui::test]
fn the_field_inherits_the_controls_radius_and_paints_no_background(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = find(&records, input::ID_CONTROL);
    let field = find(&records, input::ID_FIELD);

    assert_px(control.radius, Theme::DARK.radius_lg.value());
    assert_px(field.radius, control.radius);
    assert_px(control.radius, px(10.0));

    // `bg-background`/`dark:bg-input/32` is the control's; the field's own
    // background is `transparent` in the reference, which both extractors
    // report as no paint.
    assert_ne!(control.background, Paint::None);
    assert_eq!(field.background, Paint::None);
}

/// **The two colours the reference reports**, pinned as the tokens they come
/// out of rather than as the hex the capture happens to carry.
///
/// `/tmp/p3-ref-input.json` says `bg #ffffff07` and `border.color #ffffff14`
/// on the control. Both are `--input` — `oklch(1 0 0 / 8%)`, so `0.08 × 255`
/// is 20 = `0x14` — with the background mixed down to 32% of it, `0.0256 ×
/// 255` = 7. Asserting the arithmetic rather than the hex is what stops this
/// becoming a copy of the reference's own answer.
#[gpui::test]
fn the_controls_two_colours_are_both_the_input_token(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = find(&records, input::ID_CONTROL);
    assert_eq!(
        control.background,
        Paint::Solid(
            Theme::DARK
                .input
                .mix(input::DARK_BACKGROUND_ALPHA, crowbar_ui::Color::TRANSPARENT)
                .value()
        ),
    );
    assert_eq!(control.border_color, Some(Theme::DARK.input.value()));

    // The field paints neither, which is why its `border.color` is the one
    // field the differ ignores — v1.3 compares it only when `w > 0`.
    let field = find(&records, input::ID_FIELD);
    assert_eq!(field.background, Paint::None);
    assert_px(field.border_width, px(0.0));
}

/// The breakpoint takes one `--spacing` step off the field in every size,
/// and the control follows it because it has no height of its own.
#[gpui::test]
fn the_breakpoint_moves_the_field_and_the_control_follows(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for size in ALL_SIZES {
        let wide = measure(cx, cell(&["--size", size.name()]));
        let narrow = measure(
            cx,
            cell(&["--size", size.name(), "--viewport-width", "600"]),
        );

        let wide_field = at(&wide, input::ID_FIELD);
        let narrow_field = at(&narrow, input::ID_FIELD);
        assert_px(narrow_field.size.height - wide_field.size.height, px(4.0));

        assert_px(
            at(&wide, input::ID_CONTROL).size.height,
            wide_field.size.height + input::BORDER_WIDTH * 2.0,
        );
        assert_px(
            at(&narrow, input::ID_CONTROL).size.height,
            narrow_field.size.height + input::BORDER_WIDTH * 2.0,
        );
    }
}

/// **The call site's `ps-5` is what moves the field inside the control**, and
/// dropping it puts the field one border pixel from the edge.
///
/// The measurement behind `--class-ps` being a parameter at all: without it
/// the port would draw the primitive's control and the reference draws the
/// tree filter's, and the delta would be 20px on the field's `x` and `w`.
#[gpui::test]
fn the_call_sites_leading_pad_moves_the_field(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let with = at(&measure(cx, cell(&[])), input::ID_FIELD);
    let without = at(&measure(cx, cell(&["--class-ps", "none"])), input::ID_FIELD);

    assert_px(without.origin.x, input::BORDER_WIDTH);
    assert_px(with.origin.x - without.origin.x, px(20.0));
    assert_px(without.size.width - with.size.width, px(20.0));

    // The control is untouched by it — `ps-*` is padding, not a box change.
    assert_px(
        at(&measure(cx, cell(&[])), input::ID_CONTROL).size.width,
        at(
            &measure(cx, cell(&["--class-ps", "none"])),
            input::ID_CONTROL,
        )
        .size
        .width,
    );
}

/// **The string the field paints reaches no anchor**, however long it is.
///
/// The measurement behind the whole item. If the run were routed through
/// `AnchorSink::text_half` it would arrive here as `RawAnchor::text`, and the
/// reference — which has no text node to read — would carry none, producing
/// five `FieldPresence` deltas per cell. It is also what licenses ignoring
/// the content axis: neither anchor's box moves with the string.
#[gpui::test]
fn no_anchor_on_this_surface_reports_text(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut boxes = Vec::new();
    for line in [
        vec!["--content", "short"],
        vec!["--content", "normal"],
        vec!["--content", "overflow"],
        vec!["--value", "--content", "overflow"],
    ] {
        let records = measure(cx, cell(&line));
        for record in &records {
            assert!(record.text.is_none(), "{line:?}: {} paints text", record.id);
            assert!(!record.content_sized, "{line:?}: {}", record.id);
            assert!(!record.line_sized, "{line:?}: {}", record.id);
        }
        boxes.push((
            at(&records, input::ID_CONTROL),
            at(&records, input::ID_FIELD),
        ));
    }

    // Every content length is the same picture, on both anchors.
    for pair in boxes.windows(2) {
        assert_px(pair[0].0.size.width, pair[1].0.size.width);
        assert_px(pair[0].1.size.width, pair[1].1.size.width);
        assert_px(pair[0].1.size.height, pair[1].1.size.height);
    }
}

/// **A field's box height equals its own line height, at every size and both
/// breakpoints — and it is still not `line_sized`.**
///
/// Measured in a real window rather than asserted off the constants, because
/// the point is that taffy lays the box out at the authored height while gpui
/// shapes the run on a line box of the same number. The near-miss is what
/// `input::LINE_SIZED`'s doc comment records: two authored declarations that
/// agree are not a height *derived from* a line box, and the reference emits
/// no `font` for an `<input>` for the comparison to run against.
#[gpui::test]
fn a_fields_box_is_its_authored_height_not_a_derived_line_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for size in ALL_SIZES {
        for viewport in ["1714", "600"] {
            let records = measure(
                cx,
                cell(&["--size", size.name(), "--viewport-width", viewport]),
            );
            let field = at(&records, input::ID_FIELD);
            let breakpoint = cell(&["--viewport-width", viewport]).breakpoint();

            assert_px(field.size.height, size.extent(breakpoint));
            assert_px(field.size.height, size.line_height(breakpoint));
            // And the anchor is undeclared, which is the half a reader would
            // otherwise have to take on trust.
            assert!(!find(&records, input::ID_FIELD).line_sized);
        }
    }

    // The `sm` size at `sm:` is the live cell, and it is the 26 the app
    // reports.
    assert_px(Size::Sm.extent(cell(&[]).breakpoint()), px(26.0));
}

/// **Nothing on this surface leaks another surface's anchors**, at any cell
/// it can be driven to.
#[gpui::test]
fn no_cell_of_this_surface_records_a_foreign_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [
        vec!["--icon", "--value"],
        vec!["--size", "lg", "--flags", "focus"],
        vec!["--disabled", "--invalid", "--class-ps", "none"],
    ] {
        let seen = ids(&measure(cx, cell(&line)));
        assert!(
            seen.iter()
                .all(|id| id == input::ID_CONTROL || id == input::ID_FIELD),
            "{line:?}: {seen:?}",
        );
    }
}
