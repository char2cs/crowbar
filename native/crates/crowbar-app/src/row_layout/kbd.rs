//! `--surface kbd`: what taffy resolves a keycap to, in a real window, against
//! the captured reference at `/tmp/p3-ref-kbd.json`.
//!
//! The numbers below are the reference's: `27.61 × 20` for the `Esc` cap, a
//! 20px `min-w-5` floor, `radius 4` and — the one that would be easy to get
//! wrong — **`border_width` 0**.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::kbd;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "kbd"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the cap itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, kbd::ID_KBD, id)
}

/// **The default cell is the captured `Esc` cap** — everything about it that
/// this harness can see.
///
/// The width is asserted as its **composition** — `ceil(advance) + 2 × px-1`,
/// floored by `min-w-5` — and not against the reference's 27.61. That is
/// `badge`'s established pattern here and it is not a dodge: `add_fonts` is
/// called by `main.rs` at startup and **not** by this test harness, so a
/// headless `gpui::test` shapes with a system fallback and its advance widths
/// are its own. Measured: this cell comes out 30px wide where `ceil(27.61)` is
/// 28, a 2px difference that is entirely the missing font file.
///
/// Comparing the run against the reference is the **oracle's** job, where the
/// binary has the real face loaded. What belongs here is the arithmetic around
/// the run, which is what a port can get wrong independently of shaping.
#[gpui::test]
fn the_default_cell_is_the_captured_esc_cap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["kbd".to_owned()]);

    let root = at(&records, "kbd");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.height, px(20.0));

    let record = find(&records, "kbd");
    let advance = record.text.clone().expect("the Esc cap paints a run").width;
    // gpui ceils a run's max-content width, which is exactly what `ANCHORS.md`
    // v1.5 makes the differ compare against on the other side.
    let padded = advance.ceil() + px(4.0) * 2.0;
    assert_px(root.size.width, padded.max(px(20.0)));
    assert!(
        padded > px(20.0),
        "the Esc cap overruns min-w-5: {padded:?}"
    );

    assert_px(record.radius, px(4.0));
    assert!(record.visible);
    assert!(
        matches!(record.background, Paint::Solid(_)),
        "bg-muted paints",
    );

    let text = record.text.expect("the Esc cap paints a run");
    assert_eq!(text.content, "Esc");
    assert!(!text.clipped);
    assert_eq!(text.font.family, "CalSansUI", "the DECLARED family, v1.2");
}

/// **`border_width` is 0**, which is the mirror of `badge`'s trap and the single
/// most likely thing for a port to carry across by habit. Read off the recorded
/// anchor rather than off the constant, so a stray `.border_1()` fails here.
#[gpui::test]
fn a_keycap_pays_no_border_pixel(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    assert_px(find(&records, "kbd").border_width, px(0.0));
}

/// The height is **authored**, not derived: it stays 20 at every content length
/// while the width moves. That is the measurement behind refusing to declare
/// `line_sized`, and it is a control as much as an assertion — if the height
/// tracked the run, the declaration list would be wrong.
#[gpui::test]
fn the_height_is_authored_and_only_the_width_moves(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut widths = Vec::new();
    for content in ["short", "normal", "overflow"] {
        let records = measure(cx, cell(&["--content", content]));
        let root = at(&records, "kbd");
        assert_px(root.size.height, px(20.0));
        widths.push(f32::from(root.size.width));
    }

    // `K` is narrower than the floor, so the floor binds and the cap is square.
    assert!(
        (widths[0] - 20.0).abs() < f32::EPSILON,
        "min-w-5 should floor the short cap at 20, got {}",
        widths[0],
    );
    // And the other two overrun it, strictly increasing.
    assert!(widths[1] > widths[0], "{widths:?}");
    assert!(widths[2] > widths[1], "{widths:?}");
}

/// An icon cap sits exactly on the `min-w-5` floor and paints **no run** — three
/// of the four live caps are this shape.
#[gpui::test]
fn an_icon_cap_is_square_on_the_min_width_floor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--glyph"]));

    let root = at(&records, "kbd");
    assert_px(root.size.width, px(20.0));
    assert_px(root.size.height, px(20.0));
    assert!(
        find(&records, "kbd").text.is_none(),
        "a glyph cap paints no text of its own",
    );
}

/// §8.3's `empty`: a cap with no child at all. Same box as the icon cap, and
/// still no run — the cell that catches a port painting a run where the DOM has
/// none. `empty` overrides `--glyph`, which is what the word means.
#[gpui::test]
fn the_empty_cell_is_a_bare_cap_on_its_floor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec!["--flags", "empty"],
        vec!["--flags", "empty", "--glyph"],
    ] {
        let records = measure(cx, cell(&args));
        let root = at(&records, "kbd");
        assert_px(root.size.width, px(20.0));
        assert_px(root.size.height, px(20.0));
        assert!(find(&records, "kbd").text.is_none(), "{args:?}");
    }
}

/// `--group` changes the cap's layout context and **not** the anchor set: the
/// group carries no id, so the snapshot still holds exactly one record. That is
/// `ANCHORS.md` v1.8 — two `kbd` ids under one root would be a refusal.
#[gpui::test]
fn a_group_adds_no_anchor_and_leaves_the_cap_at_the_origin(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--group"]));

    assert_eq!(
        ids(&records),
        vec!["kbd".to_owned()],
        "the group is not an anchor, and the second cap is not the root's",
    );
    let root = at(&records, "kbd");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.size.height, px(20.0));
}
