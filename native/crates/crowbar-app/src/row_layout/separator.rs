//! `--surface separator`: what taffy resolves a one-pixel rule to, in a real
//! window.
//!
//! **There is no reference to compare these against** — every `<Separator>` in
//! the tree is Plate chrome behind a focused-editor gate this webview cannot
//! satisfy, and the live count was 0. So these assertions pin what the *port*
//! does against the app's compiled CSS, which is the most that can be claimed
//! here and is worth claiming: `self-stretch` is the one property this component
//! has that a port can get wrong, and taffy's `AlignSelf::Stretch` and CSS's are
//! two implementations of the same word.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::separator;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "separator"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the rule itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, separator::ID_SEPARATOR, id)
}

/// The default cell is `ToolbarGroup`'s vertical rule: one pixel wide, and
/// **stretched to its host** rather than collapsed. The host is 24px.
#[gpui::test]
fn the_default_cell_is_a_stretched_vertical_rule(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec!["separator".to_owned()],
        "the host row carries no anchor",
    );

    let root = at(&records, "separator");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(1.0));
    assert_px(root.size.height, px(24.0));

    let record = find(&records, "separator");
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert!(matches!(record.background, Paint::Solid(_)), "bg-border");
    assert!(record.text.is_none(), "a rule paints no text");
}

/// A horizontal rule is the transpose: one pixel tall, and `w-full` across the
/// host — so `--width` reaches it where it cannot reach the vertical arm.
#[gpui::test]
fn a_horizontal_rule_is_one_pixel_tall_and_full_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [200_u16, 400] {
        let records = measure(
            cx,
            cell(&["--orientation", "horizontal", "--width", &width.to_string()]),
        );
        let root = at(&records, "separator");
        assert_px(root.size.height, px(1.0));
        assert_px(root.size.width, px(f32::from(width)));
    }
}

/// **The vertical arm is one pixel at every width**, which is what makes
/// `--width` vacuous on it — the control for the test above.
#[gpui::test]
fn a_vertical_rule_ignores_the_width_axis(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [200_u16, 400] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        assert_px(at(&records, "separator").size.width, px(1.0));
    }
}

/// §8.3's `empty`: a host with no content of its own, so the flex line's cross
/// size is the rule's own `auto` and a stretched rule **collapses to zero
/// height** and is not painted.
///
/// This is the branch that would catch a port spelling `self-stretch` as a
/// pinned height: a pinned height cannot collapse.
#[gpui::test]
fn an_empty_host_collapses_a_stretched_rule(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));

    let root = at(&records, "separator");
    assert_px(root.size.width, px(1.0));
    assert_px(root.size.height, px(0.0));
    assert!(
        !find(&records, "separator").visible,
        "a zero-area box paints nothing, and ANCHORS.md §3 says so",
    );
}

/// A **horizontal** rule does not collapse in the same cell: its height is
/// `h-px`, authored, and `self-stretch` never applied to it. The control that
/// keeps the test above about `self-stretch` rather than about `empty`.
#[gpui::test]
fn an_empty_host_leaves_a_horizontal_rule_alone(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(
        cx,
        cell(&["--orientation", "horizontal", "--flags", "empty"]),
    );

    assert_px(at(&records, "separator").size.height, px(1.0));
    assert!(find(&records, "separator").visible);
}

/// No live call site pins a height, so every one of them stretches — the
/// vocabulary exists to make that branch expressible, and this records that it
/// is currently unexercised by the product.
#[gpui::test]
fn every_modelled_call_site_stretches(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for site in ["none", "toolbar-group", "link-toolbar"] {
        let records = measure(cx, cell(&["--call-site", site]));
        assert_px(at(&records, "separator").size.height, px(24.0));
    }
}
