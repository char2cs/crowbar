//! `--surface sidebar-toggle-icon`: what taffy resolves the panel glyph to,
//! against the captured reference at `/tmp/p3-ref-sidebar-toggle-icon.json`.
//!
//! The reference's numbers are `16 × 16`, `bg #00000000`, **`radius 0`**,
//! `border.w 0`, `visible: true`. The radius is the one worth naming: the
//! glyph's own rounded rect carries `rx="2.5"`, which is viewBox geometry and
//! **not** a CSS corner — a port that translated it would put a real 2.5px
//! radius on the box and the differ would call it.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::surfaces::sidebar::sidebar_toggle_icon;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "sidebar-toggle-icon"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the glyph itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, sidebar_toggle_icon::ID_SIDEBAR_TOGGLE_ICON, id)
}

/// **The default cell is the captured sidebar-header toggle**, and `radius 0`
/// is asserted off the recorded anchor rather than off the constant — so a stray
/// `.rounded(px(2.5))` fails here.
#[gpui::test]
fn the_default_cell_is_the_captured_sidebar_toggle(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["sidebar-toggle-icon".to_owned()]);

    let root = at(&records, "sidebar-toggle-icon");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let record = find(&records, "sidebar-toggle-icon");
    assert!(record.visible);
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert_eq!(record.background, Paint::None);
    assert!(
        record.text.is_none(),
        "a stroked <svg> paints no run, so the anchor carries no text group",
    );
    assert!(!record.content_sized);
    assert!(!record.line_sized);
}

/// **The `size-4` opt-out survives the breakpoint**, which is this component's
/// whole design. Measured in a real window at both viewport widths, against the
/// button rule it escapes — `search-toggle-icons` renders the *same* rule at 18
/// below `sm`, so the two surfaces together are the control for each other.
#[gpui::test]
fn the_extent_does_not_move_across_the_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for viewport in ["1714", "500"] {
        let records = measure(cx, cell(&["--viewport-width", viewport]));
        let root = at(&records, "sidebar-toggle-icon");
        assert_px(root.size.width, px(16.0));
        assert_px(root.size.height, px(16.0));
    }
}

/// Every modelled call site renders the same 16px box, because none of them
/// passes a className to the glyph. The record of a measurement, not a
/// tautology: the branch exists in the button's CSS and nothing in the app takes
/// it.
#[gpui::test]
fn every_call_site_renders_the_same_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for call_site in ["none", "tab-navigation", "sidebar-project-header"] {
        let records = measure(cx, cell(&["--call-site", call_site]));
        let root = at(&records, "sidebar-toggle-icon");
        assert_px(root.size.width, px(16.0));
        assert_px(root.size.height, px(16.0));
    }
}

/// §8.3's `empty`: `size-4` merged away. No area, and `visible` says so.
#[gpui::test]
fn the_empty_cell_has_no_area_and_reports_invisible(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));

    let root = at(&records, "sidebar-toggle-icon");
    assert_px(root.size.width, px(0.0));
    assert_px(root.size.height, px(0.0));
    assert!(!find(&records, "sidebar-toggle-icon").visible);
}

/// **Every recorded field is theme-invariant**: the stroke reaches the contract
/// through no field, so the two appearances record the same box.
#[gpui::test]
fn the_two_appearances_record_the_same_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark = find(
        &measure(cx, cell(&["--theme", "dark"])),
        "sidebar-toggle-icon",
    );
    let light = find(
        &measure(cx, cell(&["--theme", "light"])),
        "sidebar-toggle-icon",
    );
    assert_px(dark.bounds.size.width, light.bounds.size.width);
    assert_px(dark.bounds.size.height, light.bounds.size.height);
    assert_eq!(dark.background, light.background);
    assert_px(dark.radius, light.radius);
    assert_eq!(dark.visible, light.visible);
}
