//! `--surface scroll-area`, laid out in a real window.
//!
//! What this file has to establish that no other one does: that the **two boxes
//! land on top of each other**. The root and the viewport measured *identical*
//! on the live app — `344 × 936` at the same origin — and a coincidence like
//! that is exactly the shape a port can reproduce for the wrong reason. `h-full`
//! inside a box with no padding and no border genuinely is the whole box; a
//! viewport that had picked up a stray inset, or a root that had grown one,
//! would still look plausible in a screenshot and would be caught only here.
//!
//! The measurements the assertions are written against were taken off the
//! **running React app** at a 1714px viewport, dark, from `workspace-tree`'s
//! `<ScrollArea className="flex-1">` in the sidebar's Workspaces panel:
//!
//! ```text
//! root      344 × 936   border 0  radius 0  bg rgba(0,0,0,0)  position relative
//! viewport  344 × 936   border 0  radius 0  bg rgba(0,0,0,0)  overflow scroll
//! ```
//!
//! Two more instances were measured in the same frame and agree on every field
//! but the extent: `git-panel`'s at `344 × 920` and the command palette's at
//! `574 × 46`. That is what makes the extent a parameter and everything else a
//! constant.
//!
//! # `--theme` is **vacuous here**, and that is measured rather than assumed
//!
//! Both anchored boxes are transparent, unbordered, unrounded and paint no text,
//! so there is no field for the two tables to differ on. The one thing that does
//! move — the thumb's `bg-foreground/20` — is on an element that carries no
//! anchor. [`the_two_themes_paint_the_same_two_boxes`] asserts the vacuity
//! rather than leaving it to be discovered, because a theme cell that quietly
//! compares equal is indistinguishable from one that converged.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::scroll_area;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
///
/// Every cell is driven at the reference's own `--width`, which on this surface
/// is not a formality: the root is `size-full`, so `--width` *is* both anchors'
/// width.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "scroll-area", "--width", "344"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, scroll_area::ID_ROOT, id)
}

/// **Both contract anchors are there, and nothing else is.**
///
/// The second half matters more than usual here: this surface's root *contains*
/// the call site's whole subtree in the DOM, which is the case `ANCHORS.md` v1.8
/// was written for. The port renders an unanchored body, and this is what says
/// so.
#[gpui::test]
fn the_surface_carries_exactly_its_two_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in ["scroll-area-root", "scroll-area-viewport"] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    assert_eq!(seen.len(), 2, "{seen:?}");
}

/// **The root is the reference's box, to the pixel.**
#[gpui::test]
fn the_root_is_the_measured_three_forty_four_by_nine_thirty_six(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = at(&records, "scroll-area-root");

    assert_px(root.size.width, px(344.0));
    assert_px(root.size.height, px(936.0));
    // The root sits at the origin of its own space, by construction (§4).
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
}

/// **The viewport *is* the root**, on all four numbers — the coincidence this
/// file exists to pin.
///
/// `h-full` with no padding, no border and no radius leaves nothing between the
/// two boxes, so a single stray inset anywhere in the port shows up here as a
/// 1px offset and nowhere else.
#[gpui::test]
fn the_viewport_covers_the_root_exactly(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = at(&records, "scroll-area-root");
    let viewport = at(&records, "scroll-area-viewport");

    assert_px(viewport.origin.x, px(0.0));
    assert_px(viewport.origin.y, px(0.0));
    assert_px(viewport.size.width, root.size.width);
    assert_px(viewport.size.height, root.size.height);
}

/// **Neither box pays a border or a radius pixel**, and both are transparent —
/// which is the reading `kbd` shares and `keybinding` inverts one module over.
///
/// `border.w` is the field `ANCHORS.md` v1.1 compares *exactly*, so a port that
/// carried a border across would fail every cell.
#[gpui::test]
fn neither_box_has_a_border_a_radius_or_a_background(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for id in ["scroll-area-root", "scroll-area-viewport"] {
        let record = find(&records, id);
        assert_px(record.border_width, px(0.0));
        assert_px(record.border_width, scroll_area::BORDER_WIDTH);
        assert_px(record.radius, scroll_area::RADIUS);
        // `Paint::None` is what the driver records for an element that sets no
        // background, and it serialises to the reference's own `#00000000` —
        // the same pair `popover`'s viewport reports.
        assert_eq!(record.background, Paint::None, "{id}");
    }
}

/// `--width` moves the root **and** the viewport, together — the property that
/// says the viewport is `h-full` inside the root rather than a box with a width
/// of its own.
#[gpui::test]
fn the_width_axis_reaches_the_viewport_through_the_root(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [200u16, 344, 574] {
        let records = measure(
            cx,
            a_cell(&["--surface", "scroll-area", "--width", &width.to_string()]),
        );
        let expected = px(f32::from(width));
        assert_px(at(&records, "scroll-area-root").size.width, expected);
        assert_px(at(&records, "scroll-area-viewport").size.width, expected);
    }
}

/// `--area-height` does the same on the other axis, and the window follows it —
/// which is what stops a tall area being cut by a window sized for a short one.
#[gpui::test]
fn the_area_height_reaches_the_viewport_through_the_root(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for height in [46u16, 920, 936] {
        let records = measure(cx, cell(&["--area-height", &height.to_string()]));
        let expected = px(f32::from(height));
        assert_px(at(&records, "scroll-area-root").size.height, expected);
        assert_px(at(&records, "scroll-area-viewport").size.height, expected);
    }
}

/// **A mounted scrollbar adds no anchor and moves no box.**
///
/// This is `ANCHORS.md` v1.8 made mechanical: the tracks' presence is a function
/// of the cell, so the declared set must not contain them — and a port that
/// anchored one would produce a snapshot the reference cannot match in any
/// reachable state.
#[gpui::test]
fn an_overflowing_axis_mounts_a_track_that_carries_no_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&[]));
    for spelling in ["y", "x", "both"] {
        let records = measure(cx, cell(&["--overflow", spelling]));
        assert_eq!(ids(&records), ids(&resting), "--overflow {spelling}");

        // And the two anchored boxes are the resting ones, not merely the same
        // *set* of ids.
        for id in ["scroll-area-root", "scroll-area-viewport"] {
            assert_px(at(&records, id).size.width, at(&resting, id).size.width);
            assert_px(at(&records, id).size.height, at(&resting, id).size.height);
        }
    }
}

/// The gutter is **padding**, so it lands inside the viewport's border box and
/// the anchor does not move — which is worth pinning rather than assuming,
/// because a port that reached for a margin instead would shrink the box.
#[gpui::test]
fn the_gutter_pads_inside_the_viewports_border_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let plain = measure(cx, cell(&["--overflow", "both"]));
    let gutter = measure(cx, cell(&["--overflow", "both", "--gutter"]));

    let bare = at(&plain, "scroll-area-viewport");
    let padded = at(&gutter, "scroll-area-viewport");
    assert_px(padded.size.width, bare.size.width);
    assert_px(padded.size.height, bare.size.height);
    assert_px(padded.origin.x, bare.origin.x);
}

/// **`empty` drops every track and leaves both boxes where they were** — the
/// cell the surface models and the caption qualifies.
#[gpui::test]
fn the_empty_flag_leaves_both_boxes_untouched(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&[]));
    let emptied = measure(cx, cell(&["--overflow", "both", "--flags", "empty"]));

    assert_eq!(ids(&emptied), ids(&resting));
    for id in ["scroll-area-root", "scroll-area-viewport"] {
        assert_px(at(&emptied, id).size.width, at(&resting, id).size.width);
        assert_px(at(&emptied, id).size.height, at(&resting, id).size.height);
    }
}

/// **The two tables paint the same two boxes**, which makes `--theme` vacuous on
/// this surface — asserted rather than discovered.
///
/// A theme cell that quietly compares equal looks exactly like one that
/// converged. Every field the contract carries for these two anchors is
/// table-independent, and the one colour that is not — the thumb's — is on an
/// element with no anchor. The control is the last line: the tables *are*
/// different, so this is a fact about these boxes rather than about the theme.
#[gpui::test]
fn the_two_themes_paint_the_same_two_boxes(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark = measure(cx, cell(&["--theme", "dark"]));
    let light = measure(cx, cell(&["--theme", "light"]));

    for id in ["scroll-area-root", "scroll-area-viewport"] {
        let a = find(&dark, id);
        let b = find(&light, id);
        assert_eq!(a.background, b.background, "{id}");
        assert_px(a.radius, b.radius);
        assert_px(a.border_width, b.border_width);
        assert_px(at(&dark, id).size.width, at(&light, id).size.width);
        assert_px(at(&dark, id).size.height, at(&light, id).size.height);
    }

    // The control: the tables genuinely differ, so the equality above is a
    // property of these boxes and not of the theme selector.
    assert_ne!(Theme::LIGHT.foreground, Theme::DARK.foreground);
}

/// Neither anchor declares `content_sized` or `line_sized`, and the declarations
/// travel — `ANCHORS.md` v1.5 and v1.6 make both properties the *component*
/// asserts and the differ trusts, so one that silently stopped travelling would
/// open a blind spot and announce nothing.
#[gpui::test]
fn neither_anchor_declares_a_sizing_property(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for id in ["scroll-area-root", "scroll-area-viewport"] {
        let record = find(&records, id);
        assert!(!record.content_sized, "{record:?}");
        assert!(!record.line_sized, "{record:?}");
        // And neither paints a run, which is what makes `line_sized` refusable
        // rather than merely false.
        assert!(record.text.is_none(), "{record:?}");
    }
}
