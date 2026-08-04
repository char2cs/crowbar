//! `--surface loading-spinner`: what taffy resolves a glyph, a gap and a
//! caption to, in a real window.
//!
//! The reference is `138 × 18` overall, `16 × 16` at `y 1` for the glyph and
//! `116 × 18` at `x 22` for the caption.
//!
//! **The caption's advance width is not asserted against the reference's**, for
//! `label`'s already-recorded reason: `add_fonts` is called by `main.rs` and not
//! by this harness, so a headless `gpui::test` shapes with a system fallback and
//! its advance widths are its own. Measured, this cell's run comes out
//! **136.8** where the reference's is 115.99 — 18% wide, and every pixel of it
//! the missing font file. Comparing the run against the reference is the
//! **oracle's** job, where the binary has the real face loaded.
//!
//! What *is* asserted exactly is everything the face cannot move: the gap, the
//! glyph's box, the cross-axis centring, the 18px line box, and the wrapper as
//! the arithmetic sum of the row.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::primitives::loading_spinner;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "loading-spinner"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the wrapper.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, loading_spinner::ID_ROOT, id)
}

/// The default cell is the captured commit-diff spinner: three anchors, the
/// glyph centred against an 18px line box, and the wrapper the sum of the row.
#[gpui::test]
fn the_default_cell_is_the_captured_commit_diff_row(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec![
            "loading-spinner".to_owned(),
            "spinner".to_owned(),
            "loading-spinner-label".to_owned(),
        ],
        "the centred column carries no anchor",
    );

    let glyph = at(&records, "spinner");
    assert_px(glyph.origin.x, px(0.0));
    assert_px(glyph.size.width, px(16.0));
    assert_px(glyph.size.height, px(16.0));
    // `items-center` against the caption's 18px line box: (18 − 16) / 2.
    assert_px(glyph.origin.y, px(1.0));

    let caption = at(&records, "loading-spinner-label");
    // 16 + `gap-1.5`'s 6, which is the reference's 22 exactly.
    assert_px(caption.origin.x, px(22.0));
    assert_px(caption.origin.y, px(0.0));
    assert_px(caption.size.height, px(18.0));

    let record = find(&records, "loading-spinner-label");
    let text = record.text.clone().expect("the caption paints a run");
    // The caption's box *is* its run: no padding, no border, no authored width.
    assert_px(caption.size.width, text.width.ceil());

    let wrapper = at(&records, "loading-spinner");
    assert_px(wrapper.origin.x, px(0.0));
    assert_px(wrapper.origin.y, px(0.0));
    assert_px(wrapper.size.height, px(18.0));
    // And the wrapper is the whole row: glyph + gap + caption, with nothing of
    // its own. That is what makes the reference's 138 = 16 + 6 + 116.
    assert_px(wrapper.size.width, caption.origin.x + caption.size.width);

    assert_eq!(text.content, "Loading commit diff");
    assert!(!text.clipped, "nothing truncates a centred caption");
    // The reference's font group: 12px CalSansUI at weight 400 on an 18px line
    // box — preflight's `line-height: 1.5`, nothing on the caption authoring one.
    assert_px(text.font.size, px(12.0));
    assert_px(text.font.line_height, px(18.0));
    assert!(
        (text.font.weight - 400.0).abs() < f32::EPSILON,
        "weight 400"
    );
    assert_eq!(text.font.family, "CalSansUI");
}

/// The wrapper and the caption paint **nothing**: no background, no radius, no
/// border. `loading-spinner.tsx` carries no `border` class, so preflight's
/// `border: 0 solid` stands — `kbd`'s side of that trap rather than `badge`'s.
#[gpui::test]
fn no_anchor_on_this_surface_paints_a_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for id in ["loading-spinner", "spinner", "loading-spinner-label"] {
        let record = find(&records, id);
        assert_eq!(record.background, Paint::None, "{id}");
        assert_px(record.radius, px(0.0));
        assert_px(record.border_width, px(0.0));
        assert!(record.visible, "{id}");
    }
}

/// `compact` moves the gap **and** the glyph together, which is one prop with
/// two consequences: 12 + 4 against 16 + 6 puts the caption 6px further left.
#[gpui::test]
fn compact_narrows_both_the_glyph_and_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let full = measure(cx, cell(&["--call-site", "branch-diff"]));
    assert_px(at(&full, "spinner").size.width, px(16.0));
    assert_px(at(&full, "loading-spinner-label").origin.x, px(22.0));

    let compact = measure(cx, cell(&["--call-site", "connecting-row"]));
    assert_px(at(&compact, "spinner").size.width, px(12.0));
    assert_px(at(&compact, "loading-spinner-label").origin.x, px(16.0));
}

/// §8.3's `empty` **drops an anchor** rather than a field, which is the loudest
/// difference the differ has — and it lands on the same picture
/// `--call-site fallback` reaches from the other direction.
#[gpui::test]
fn the_empty_cell_drops_the_caption_and_agrees_with_the_fallback(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let empty = measure(cx, cell(&["--flags", "empty"]));
    let fallback = measure(cx, cell(&["--call-site", "fallback"]));

    for records in [&empty, &fallback] {
        assert_eq!(
            ids(records),
            vec!["loading-spinner".to_owned(), "spinner".to_owned()],
            "no caption, so no caption anchor",
        );
        // The wrapper collapses onto the glyph: the gap needs two items to show.
        let wrapper = relative_to(records, loading_spinner::ID_ROOT, loading_spinner::ID_ROOT);
        assert_px(wrapper.size.width, px(16.0));
        assert_px(wrapper.size.height, px(16.0));
    }
}
