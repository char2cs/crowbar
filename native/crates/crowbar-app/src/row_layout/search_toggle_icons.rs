//! `--surface search-toggle-icons`: what taffy resolves the four toggle icons
//! to, against the two captured references —
//! `/tmp/p3-ref-search-toggle-icons.json` (the `preserve-case` run) and
//! `/tmp/p3-ref-search-toggle-icons-glyph.json` (a phosphor cell).
//!
//! The run's numbers are `14.48 × 15`, `text_width 14.48`, `CalSansUI` 11/15.71
//! at weight 600. The glyph's are `16 × 16` and nothing else — which is the
//! asymmetry these tests exist to pin.
//!
//! **The run's width is asserted as its composition**, `badge`'s and `kbd`'s
//! established pattern here: `add_fonts` is called by `main.rs` at startup and
//! **not** by this harness, so a headless `gpui::test` shapes with a system
//! fallback and its advance widths are its own. Comparing the run against the
//! reference is the oracle's job, where the binary has the real face loaded.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::surfaces::search_toggle_icons;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "search-toggle-icons"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the icon itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, search_toggle_icons::ID_SEARCH_TOGGLE_ICON, id)
}

/// **The default cell is the captured `preserve-case` run** — the only shape on
/// this surface the contract can see more than a box of.
#[gpui::test]
fn the_default_cell_is_the_captured_preserve_case_run(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["search-toggle-icon".to_owned()]);

    let root = at(&records, "search-toggle-icon");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));

    let record = find(&records, "search-toggle-icon");
    assert!(record.visible);
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert_eq!(record.background, Paint::None);
    assert!(
        record.content_sized,
        "the box is the run's max-content width"
    );
    assert!(record.line_sized, "and its height IS the run's line box");

    let text = record.text.expect("the preserve-case cell paints a run");
    assert_eq!(text.content, "Aa");
    assert!(!text.clipped);
    assert_eq!(text.font.family, "CalSansUI", "the DECLARED family, v1.2");
    assert_px(text.font.size, px(11.0));
    assert!(
        (text.font.weight - 600.0).abs() < f32::EPSILON,
        "font-semibold is 600, got {}",
        text.font.weight,
    );

    // The box is the run's, on both axes: gpui ceils a run's max-content width,
    // which is exactly what `ANCHORS.md` v1.5 makes the differ compare against
    // on the other side.
    assert_px(root.size.width, text.width.ceil());
    assert_px(root.size.height, text.font.line_height);
}

/// **The line box is the inherited ratio**, and it is what makes `line_sized`
/// worth declaring here: `11 × 1.25/0.875` is `15.714`, which the reference
/// floors to a `bounds.h` of 15 while the port snaps it. The `Base` arm is the
/// control — a ratio that did not move would make `--viewport-width` vacuous on
/// this shape, and it is not.
#[gpui::test]
fn the_runs_line_box_is_the_inherited_ratio_at_each_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "1714"]));
    let wide_text = find(&wide, "search-toggle-icon")
        .text
        .expect("a run at the sm step");
    assert!(
        (f32::from(wide_text.font.line_height) - 15.714_286).abs() <= 0.5,
        "sm: {:?}",
        wide_text.font.line_height,
    );

    let narrow = measure(cx, cell(&["--viewport-width", "500"]));
    let narrow_text = find(&narrow, "search-toggle-icon")
        .text
        .expect("a run at the base step");
    assert!(
        (f32::from(narrow_text.font.line_height) - 16.5).abs() <= 0.5,
        "base: {:?}",
        narrow_text.font.line_height,
    );

    assert!(
        narrow_text.font.line_height > wide_text.font.line_height,
        "the ratio moves at the breakpoint: {:?} vs {:?}",
        narrow_text.font.line_height,
        wide_text.font.line_height,
    );
    // And the font size does not — `ui-text-xs` is unprefixed.
    assert_px(narrow_text.font.size, wide_text.font.size);
}

/// **A glyph cell carries no text group at all**, and its box is the host
/// button's icon rule — 16 above `sm`, 18 below. `ANCHORS.md` ranks a missing
/// field group above a wrong number, so this is the cell most likely to catch a
/// port painting a run where the DOM has none.
#[gpui::test]
fn a_glyph_cell_is_a_bare_box_that_moves_at_the_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for toggle in ["case-sensitive", "whole-word", "regex"] {
        let wide = measure(cx, cell(&["--toggle", toggle, "--viewport-width", "1714"]));
        let record = find(&wide, "search-toggle-icon");
        assert!(record.text.is_none(), "{toggle} paints no run");
        assert!(!record.content_sized, "{toggle}");
        assert!(!record.line_sized, "{toggle}");
        assert_px(at(&wide, "search-toggle-icon").size.width, px(16.0));
        assert_px(at(&wide, "search-toggle-icon").size.height, px(16.0));

        let narrow = measure(cx, cell(&["--toggle", toggle, "--viewport-width", "500"]));
        assert_px(at(&narrow, "search-toggle-icon").size.width, px(18.0));
        assert_px(at(&narrow, "search-toggle-icon").size.height, px(18.0));
    }
}

/// The active state moves the run's colour and **nothing on a glyph**. The
/// second half is the finding: `fill: currentColor` has no field, so driving
/// `selected` on a glyph cell compares resting against resting.
#[gpui::test]
fn selected_moves_the_runs_colour_and_no_field_on_a_glyph(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = find(&measure(cx, cell(&[])), "search-toggle-icon")
        .text
        .expect("a run");
    let engaged = find(
        &measure(cx, cell(&["--flags", "selected"])),
        "search-toggle-icon",
    )
    .text
    .expect("a run");
    assert_ne!(
        resting.color, engaged.color,
        "the active toggle takes text-foreground",
    );

    let glyph_resting = find(
        &measure(cx, cell(&["--toggle", "regex"])),
        "search-toggle-icon",
    );
    let glyph_engaged = find(
        &measure(cx, cell(&["--toggle", "regex", "--flags", "selected"])),
        "search-toggle-icon",
    );
    assert_eq!(
        glyph_resting, glyph_engaged,
        "every recorded field on a glyph is the same in both states",
    );
}

/// §8.3's `empty`: the box merged away to zero on **both** shapes. The run
/// survives — the primitive owns the legend — but the declarations come off
/// with the measure, which is what `ANCHORS.md` v1.5 and v1.6 require of an
/// authored box.
#[gpui::test]
fn the_empty_cell_zeroes_the_box_and_drops_both_declarations(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let run = measure(cx, cell(&["--flags", "empty"]));
    let record = find(&run, "search-toggle-icon");
    assert_px(at(&run, "search-toggle-icon").size.width, px(0.0));
    assert_px(at(&run, "search-toggle-icon").size.height, px(0.0));
    assert!(!record.visible);
    assert!(!record.content_sized);
    assert!(!record.line_sized);
    assert!(
        record.text.is_some(),
        "the legend is the primitive's own and nothing can take it away",
    );

    let glyph = measure(cx, cell(&["--toggle", "regex", "--flags", "empty"]));
    assert_px(at(&glyph, "search-toggle-icon").size.width, px(0.0));
    assert!(!find(&glyph, "search-toggle-icon").visible);
}

/// `--theme` is real **only on the run**: `--muted-foreground` differs in the
/// two tables and reaches the contract through `fg`. On a glyph the same colour
/// reaches it through nothing, and every recorded field is identical.
#[gpui::test]
fn the_theme_moves_the_run_and_nothing_on_a_glyph(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark = find(
        &measure(cx, cell(&["--theme", "dark"])),
        "search-toggle-icon",
    )
    .text
    .expect("a run");
    let light = find(
        &measure(cx, cell(&["--theme", "light"])),
        "search-toggle-icon",
    )
    .text
    .expect("a run");
    assert_ne!(dark.color, light.color);

    let glyph_dark = find(
        &measure(cx, cell(&["--toggle", "regex", "--theme", "dark"])),
        "search-toggle-icon",
    );
    let glyph_light = find(
        &measure(cx, cell(&["--toggle", "regex", "--theme", "light"])),
        "search-toggle-icon",
    );
    assert_eq!(glyph_dark, glyph_light);
}
