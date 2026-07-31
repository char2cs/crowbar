//! `--surface label`: what taffy resolves a label to, in a real window, against
//! the captured reference at `/tmp/p3-ref-label.json`.
//!
//! The reference is `80.89 × 16` around a 16px line box at 14px — and the
//! module this measures exists because that 14 is **not** the number the call
//! site's `ui-text-sm` names. The `--viewport-width` test below is the one that
//! would catch a port reading the class list instead of the cascade.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::git_status_row::BREAKPOINT_SM;
use crowbar_ui::components::label;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "label"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the label itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, label::ID_LABEL, id)
}

/// **The default cell is the captured Typography header** — everything about it
/// that this harness can see.
///
/// The width is asserted as its **composition** — `ceil(advance)`, the box being
/// the run and nothing else — and not against the reference's 80.89. That is
/// `badge`'s established pattern here and it is not a dodge: `add_fonts` is
/// called by `main.rs` at startup and **not** by this test harness, so a
/// headless `gpui::test` shapes with a system fallback and its advance widths
/// are its own. Measured: this cell comes out 85px wide where `ceil(80.89)` is
/// 81, a 4px difference that is entirely the missing font file.
///
/// Comparing the run against the reference is the **oracle's** job, where the
/// binary has the real face loaded.
#[gpui::test]
fn the_default_cell_is_the_captured_typography_header(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["label".to_owned()]);

    let root = at(&records, "label");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    let record = find(&records, "label");
    let advance = record.text.clone().expect("a label paints its run").width;
    // The box *is* the run: no padding, no border, no authored width.
    assert_px(root.size.width, advance.ceil());

    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);

    let text = record.text.expect("a label paints its run");
    assert_eq!(text.content, "Typography");
    assert!(!text.clipped);
    assert_eq!(text.font.family, "CalSansUI");
}

/// **The box height *is* the line box**, which is what `line_sized` declares —
/// and unlike `kbd`, the two numbers agree here because nothing authors a
/// height. The reference reports `bounds.h 16` and `font.line_height 16`.
///
/// The control matters as much as the assertion: `kbd`'s box differs from its
/// line box by 4px, so "height equals line height" is a real property of this
/// component rather than something every box satisfies.
#[gpui::test]
fn the_box_height_is_its_own_line_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let height = at(&records, "label").size.height;
    let line_height = find(&records, "label")
        .text
        .expect("a label paints its run")
        .font
        .line_height;
    assert!(
        (f32::from(height) - f32::from(line_height)).abs() <= 0.5,
        "line_sized: {height:?} against {line_height:?}",
    );
}

/// **The `sm:` step wins above 640px and loses below it**, which is the whole
/// reason `--viewport-width` is a real axis here. Below the breakpoint the type
/// step is 16/18 and the box grows; at and above it, 14/16.
#[gpui::test]
fn the_type_step_moves_at_the_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "1714"]));
    let wide_font = find(&wide, "label").text.expect("a run").font;
    assert!(
        (f32::from(wide_font.size) - 14.0).abs() < 0.01,
        "sm:text-sm/4 is 14px, got {:?} — NOT the call site's 12px ui-text-sm",
        wide_font.size,
    );

    // One pixel below the breakpoint, written as a literal because the cell
    // takes whole px; the assertion below keeps it honest against the constant.
    let narrow_width = 639_u16;
    assert!(f32::from(narrow_width) < BREAKPOINT_SM);
    let narrow = measure(cx, cell(&["--viewport-width", &narrow_width.to_string()]));
    let narrow_font = find(&narrow, "label").text.expect("a run").font;
    assert!(
        (f32::from(narrow_font.size) - 16.0).abs() < 0.01,
        "text-base/4.5 is 16px, got {:?}",
        narrow_font.size,
    );

    // And the box follows the step, so the axis is not merely a font field.
    assert!(
        at(&narrow, "label").size.height > at(&wide, "label").size.height,
        "the 18px line box should be taller than the 16px one",
    );
}

/// `--content` moves the run's width and nothing else — the box is
/// content-sized, so it tracks, strictly.
#[gpui::test]
fn the_width_tracks_the_run(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut widths = Vec::new();
    for content in ["short", "normal", "overflow"] {
        let records = measure(cx, cell(&["--content", content]));
        assert_px(at(&records, "label").size.height, px(16.0));
        widths.push(f32::from(at(&records, "label").size.width));
    }
    assert!(widths[0] < widths[1], "{widths:?}");
    assert!(widths[1] < widths[2], "{widths:?}");
}

/// §8.3's `empty`: no children, so **no run at all** — and with no run the
/// anchor must not claim `line_sized`, which `ANCHORS.md` v1.6 makes a refusal
/// rather than a delta.
#[gpui::test]
fn the_empty_cell_paints_no_run_and_declares_no_line_sizing(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));

    assert_eq!(ids(&records), vec!["label".to_owned()]);
    let record = find(&records, "label");
    assert!(record.text.is_none(), "an empty label paints nothing");
    assert!(
        !record.line_sized,
        "v1.6: a box with no font may not declare line_sized",
    );
    assert!(record.content_sized, "the box still sizes to its content");
    assert_px(at(&records, "label").size.width, px(0.0));
}

/// `--call-site row` is the one live override of a visual property, and it
/// moves the **weight** and nothing structural.
#[gpui::test]
fn the_row_call_site_moves_only_the_weight(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let header = measure(cx, cell(&["--call-site", "section-header"]));
    let row = measure(cx, cell(&["--call-site", "row"]));

    let header_font = find(&header, "label").text.expect("a run").font;
    let row_font = find(&row, "label").text.expect("a run").font;
    // `FontFacts::weight` is a bare `f32`, so this is a spread rather than an
    // `assert_ne!` — 500 against 400, nowhere near a rounding question.
    assert!(
        (header_font.weight - row_font.weight).abs() > 1.0,
        "font-medium against font-normal: {:?} / {:?}",
        header_font.weight,
        row_font.weight,
    );
    assert_eq!(header_font.size, row_font.size);
    assert_px(
        at(&header, "label").size.height,
        at(&row, "label").size.height,
    );
}
