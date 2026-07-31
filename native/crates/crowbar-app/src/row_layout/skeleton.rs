//! `--surface skeleton`: what taffy resolves a loading placeholder to, in a real
//! window.
//!
//! **There is no reference to compare these against** — the only call site is a
//! `<Suspense>` fallback whose subtree contains no suspending source, so it
//! never mounts and the live count was 0. These assertions pin what the port
//! does against the app's compiled CSS.
//!
//! Nothing here needs a settling wait: the animation moves `background-position`
//! and the contract records no such field, so there is no instant at which these
//! numbers differ. See the component's module docs.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::skeleton;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "skeleton"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the placeholder itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, skeleton::ID_SKELETON, id)
}

/// The default cell is the chat row's leading icon block: `h-5 w-5 rounded-md`,
/// so 20 × 20 at the theme's 8px `--radius-md`.
#[gpui::test]
fn the_default_cell_is_the_rows_icon_block(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec!["skeleton".to_owned()],
        "the host row carries no anchor",
    );

    let root = at(&records, "skeleton");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(20.0));
    assert_px(root.size.height, px(20.0));

    let record = find(&records, "skeleton");
    assert_px(record.radius, px(8.0));
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert!(matches!(record.background, Paint::Solid(_)), "bg-muted");
    assert!(record.text.is_none(), "a placeholder paints no text");
}

/// **Every live shape overrides the primitive's `rounded-sm`**, and the three
/// radii are genuinely different numbers — which is what stops the assertion
/// being one that would pass whatever the component did.
#[gpui::test]
fn each_live_shape_paints_its_own_corner(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let icon = find(&measure(cx, cell(&["--call-site", "icon"])), "skeleton").radius;
    let meta = find(&measure(cx, cell(&["--call-site", "meta"])), "skeleton").radius;
    let bare = find(&measure(cx, cell(&["--call-site", "none"])), "skeleton").radius;

    assert_px(icon, px(8.0)); // rounded-md
    assert_px(meta, px(4.0)); // rounded
    assert_px(bare, px(6.0)); // the primitive's own rounded-sm
    assert_ne!(icon, meta);
    assert_ne!(meta, bare);
}

/// `--call-site title` is the `flex-1` shape and the only one `--width` moves;
/// the other two pin their own width and ignore it.
#[gpui::test]
fn only_the_title_shape_follows_the_width_axis(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let narrow = measure(cx, cell(&["--call-site", "title", "--width", "200"]));
    let wide = measure(cx, cell(&["--call-site", "title", "--width", "400"]));
    assert_px(at(&narrow, "skeleton").size.width, px(200.0));
    assert_px(at(&wide, "skeleton").size.width, px(400.0));
    // and its height is its own `h-3`, not the row's.
    assert_px(at(&wide, "skeleton").size.height, px(12.0));

    for site in ["icon", "meta"] {
        let a = measure(cx, cell(&["--call-site", site, "--width", "200"]));
        let b = measure(cx, cell(&["--call-site", site, "--width", "400"]));
        assert_px(at(&a, "skeleton").size.width, at(&b, "skeleton").size.width);
    }
}

/// §8.3's `empty`: the bare primitive pins neither axis, so the box has **zero
/// area** and is not painted — and it is the same cell `--call-site none`
/// reaches from the other direction, which is the check that the two spellings
/// of "nothing in it" agree.
#[gpui::test]
fn the_empty_cell_has_no_area_and_agrees_with_the_bare_call_site(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec!["--flags", "empty"],
        vec!["--call-site", "none"],
        // `empty` overrides a call site that would have pinned a box.
        vec!["--flags", "empty", "--call-site", "icon"],
    ] {
        let records = measure(cx, cell(&args));
        let root = at(&records, "skeleton");
        assert_px(root.size.width, px(0.0));
        assert_px(root.size.height, px(0.0));
        assert!(
            !find(&records, "skeleton").visible,
            "a zero-area box paints nothing: {args:?}",
        );
    }
}
