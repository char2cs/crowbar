//! `--surface sidebar-header`, laid out in a real window.
//!
//! What this file has to establish that `row_layout/popover.rs` does not: that a
//! wrapped `gpui-component` widget whose own box has been **refined down to a
//! passthrough** really does contribute nothing. `popover` turned the vendor's
//! paint off with `appearance(false)` and left it taking room; here the vendor's
//! `p_2()`, `gap_2()`, `justify_between()`, `rounded(theme.radius)` and its
//! *row* direction are all cancelled through `Styled`, and every one of those
//! would move a number if the cancellation did not land.
//!
//! The measurements the assertions are written against were taken off the
//! **running React app** at a 1714px viewport, dark, with the sidebar carousel
//! on its Files panel:
//!
//! ```text
//! sidebar-header   344 × 44   padding 8 on all four sides   radius 0   border 0
//!                             bg rgba(0,0,0,0)   gap 8
//! its single child 328 × 28   at (8, 8)
//! ```
//!
//! `/tmp/p3-ref-sidebar.json` is that capture, reduced to this surface's one
//! anchor by the `sidebar-header` entry `oracleSurfaceScope` gained on this
//! branch — the live header also contains an `<Input>` and a `<Button>`, which
//! carry `input-control`, `input` and `button` and are the **call site's**
//! anchors rather than this surface's (`ANCHORS.md` v1.8).

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::surfaces::sidebar::shell as sidebar;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "sidebar-header"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, sidebar::ID_HEADER, id)
}

/// **The wrap renders, and it renders exactly one anchor.**
///
/// The first thing to check and the thing most likely to break on a
/// `gpui-component` bump. It also pins v1.8 from the native side: the vendor's
/// `SidebarHeader` is an element with an id of its own (`"sidebar-header"`, as
/// it happens) and it must not become a second record under that name.
#[gpui::test]
fn the_wrapped_header_carries_exactly_its_own_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--width", "344"])));

    assert_eq!(seen, vec!["sidebar-header".to_owned()], "{seen:?}");
}

/// **The header box is the reference's, to the pixel** — 344 × 44 at the origin
/// of its own space.
///
/// This is the assertion the whole item turns on: the vendor's box wraps this
/// one, and if any of the five refinements failed to land it would be offset,
/// inset or laid out along the wrong axis.
#[gpui::test]
fn the_header_is_the_measured_three_forty_four_by_forty_four(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344"]));
    let header = at(&records, "sidebar-header");

    assert_px(header.size.width, px(344.0));
    assert_px(header.size.height, px(44.0));
    assert_px(header.origin.x, px(0.0));
    assert_px(header.origin.y, px(0.0));
}

/// **The vendor's `p_2()` does not land on top of ours.**
///
/// `gpui_component::sidebar::SidebarHeader::render` writes `p_2()` — the same
/// 8px `sidebar.tsx` writes — before `refine_style`, so a wrap that forgot to
/// cancel it would produce a 360 × 60 box out of the same parameters and look
/// almost right. This is the mutation that catches it, spelled as the
/// difference: 16px of padding, not 32.
#[gpui::test]
fn the_vendors_own_padding_is_cancelled_rather_than_doubled(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344"]));
    let header = at(&records, "sidebar-header");

    let padding = f32::from(sidebar::HEADER_PADDING);
    assert_px(header.size.height, px(28.0 + padding * 2.0));
    assert!((f32::from(header.size.height) - (28.0 + padding * 4.0)).abs() > 1.0);
    assert_px(sidebar::HEADER_PADDING, px(8.0));
}

/// **The header paints nothing** — no background, no border, no radius — which
/// is what the reference says and *not* what the vendor's box would say.
///
/// `SidebarHeader::render` ends with `.rounded(cx.theme().radius)`, which is
/// `gpui-component`'s radius rather than this design system's; `radius` is a
/// field `ANCHORS.md` §3 compares, and 0 is the number the live element reports.
#[gpui::test]
fn the_header_carries_no_paint_and_no_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let header = find(&measure(cx, cell(&["--width", "344"])), "sidebar-header");

    assert_px(header.radius, px(0.0));
    assert_px(header.border_width, px(0.0));
    // `Paint::None` and not a transparent `Solid`: the box authors no `bg` at
    // all, and `schema.rs` writes `#00000000` for it — the reference's own value.
    assert_eq!(header.background, Paint::None);
    // `border.color` is compared only where `border.w > 0` (ANCHORS.md v1.1),
    // so a colour here would be unobservable either way — which is exactly why
    // the width is the field to assert.
    assert!(header.text.is_none(), "{header:?}");
}

/// The box follows `--body-height` through its padding on **both** edges, which
/// is what makes the body a legitimate stand-in for the call site's children.
#[gpui::test]
fn the_box_height_follows_the_body_through_its_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for body in [0u16, 28, 96] {
        let records = measure(
            cx,
            cell(&["--width", "344", "--body-height", &body.to_string()]),
        );
        let header = at(&records, "sidebar-header");
        let expected = f32::from(sidebar::HEADER_PADDING) * 2.0 + f32::from(body);

        assert_px(header.size.height, px(expected));
        // …and the width does not move with it: it is the parent's.
        assert_px(header.size.width, px(344.0));
    }
}

/// **The header stretches to `--width` rather than shrink-wrapping**, which is
/// the reason it carries neither declaration.
///
/// The vendor's `w_full()` is what does it, and it survives the refinement — so
/// this is also the assertion that would catch a future refinement setting a
/// width by accident.
#[gpui::test]
fn the_header_takes_its_width_from_the_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [200u16, 344, 600] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        let header = find(&records, "sidebar-header");

        assert_px(header.bounds.size.width, px(f32::from(width)));
        assert!(!header.content_sized, "{header:?}");
        assert!(!header.line_sized, "{header:?}");
    }

    assert!(!sidebar::CONTENT_SIZED.contains(&sidebar::ID_HEADER));
    assert!(!sidebar::LINE_SIZED.contains(&sidebar::ID_HEADER));
}

/// `empty` collapses the box to its own padding, and to nothing else: 16px tall
/// and still the full width.
#[gpui::test]
fn the_empty_header_is_its_padding_and_nothing_else(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344", "--flags", "empty"]));
    let header = at(&records, "sidebar-header");

    assert_px(header.size.height, px(16.0));
    assert_px(header.size.width, px(344.0));
}

/// The light table paints the **same** header, which is worth an assertion
/// rather than a shrug: this box takes no token at all, so a theme cell on this
/// surface is vacuous *by construction* and the reason is `sidebar.tsx`'s, not
/// an omission in the port.
#[gpui::test]
fn the_theme_does_not_reach_this_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let dark = find(&measure(cx, cell(&["--width", "344"])), "sidebar-header");
    let light = find(
        &measure(cx, cell(&["--width", "344", "--theme", "light"])),
        "sidebar-header",
    );

    assert_eq!(dark.background, light.background);
    assert_px(dark.radius, light.radius);
    assert_px(dark.border_width, light.border_width);
    assert_eq!(dark.bounds.size, light.bounds.size);
}
