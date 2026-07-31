//! `--surface flicker-spinner`: what taffy resolves a flip-dot spinner to, in a
//! real window.
//!
//! Nothing here needs a settling wait, and that is a measurement rather than a
//! hope: every `<animate>` in `spinners/*.svg` moves `fill-opacity`, the
//! contract records no such field, and `getAnimations()` on the anchored span
//! returned `[]`. See the component's module docs.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::flicker_spinner;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "flicker-spinner"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the span itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, flicker_spinner::ID_FLICKER_SPINNER, id)
}

/// The default cell is the agent chat pane's `size-6`: 24 × 24, the box the
/// reference reports, with no paint of any kind on it.
#[gpui::test]
fn the_default_cell_is_the_captured_chat_pane_spinner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec!["flicker-spinner".to_owned()],
        "the centred column carries no anchor",
    );

    let root = at(&records, "flicker-spinner");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(24.0));
    assert_px(root.size.height, px(24.0));

    let record = find(&records, "flicker-spinner");
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert_eq!(
        record.background,
        Paint::None,
        "the span paints no background"
    );
    assert!(
        record.text.is_none(),
        "the dots are an unanchored <svg>, so the reference emits no fg either",
    );
}

/// Every live call site's box — and the one that **restates** the primitive's
/// own `size-4` rather than overriding it.
#[gpui::test]
fn each_call_site_paints_its_own_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut boxes = Vec::new();
    for site in ["none", "chat-glyph", "chat-pane", "workspace-icon"] {
        let records = measure(cx, cell(&["--call-site", site]));
        let extent = at(&records, "flicker-spinner").size;
        assert_px(extent.width, extent.height);
        boxes.push(extent.width);
    }

    assert_px(boxes[0], px(16.0)); // the primitive's own size-4
    assert_px(boxes[1], px(16.0)); // agent-chat-glyph.tsx restates it
    assert_px(boxes[2], px(24.0)); // agent-chat-pane.tsx's size-6
    assert_px(boxes[3], px(14.0)); // workspace-branch-icon.tsx's size-3.5
    // The claim that one restates the default is only worth making because the
    // other two genuinely differ from it.
    assert_ne!(boxes[0], boxes[2]);
    assert_ne!(boxes[0], boxes[3]);
}

/// `--viewport-width` is **vacuous** on this surface, and that is asserted
/// rather than assumed: neither the primitive nor any call site carries a `sm:`
/// variant, which is exactly the check P3.3's trap costs when it is skipped.
#[gpui::test]
fn the_breakpoint_moves_nothing_on_this_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for site in ["none", "chat-glyph", "chat-pane", "workspace-icon"] {
        let narrow = measure(cx, cell(&["--call-site", site, "--viewport-width", "639"]));
        let wide = measure(cx, cell(&["--call-site", site, "--viewport-width", "1714"]));
        assert_px(
            at(&narrow, "flicker-spinner").size.width,
            at(&wide, "flicker-spinner").size.width,
        );
    }
}

/// §8.3's `empty`: a zero-area box that paints nothing, from every call site.
#[gpui::test]
fn the_empty_cell_has_no_area_and_overrides_every_call_site(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for site in ["none", "chat-pane", "workspace-icon"] {
        let records = measure(cx, cell(&["--flags", "empty", "--call-site", site]));
        let root = at(&records, "flicker-spinner");
        assert_px(root.size.width, px(0.0));
        assert_px(root.size.height, px(0.0));
        assert!(
            !find(&records, "flicker-spinner").visible,
            "a zero-area box paints nothing: {site}",
        );
    }
}
