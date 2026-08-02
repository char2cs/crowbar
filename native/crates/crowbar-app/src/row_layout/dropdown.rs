//! `--surface dropdown`, laid out in a real window.
//!
//! What this file has to establish, on top of what `row_layout/popover.rs`
//! already did for the wrap mechanism itself: that this shell — one box,
//! where `popover`'s is two — comes out at exactly border-plus-padding-plus-
//! body on every axis, and that the vendor's own wrapper layer (the same
//! `v_flex().occlude().tab_group()` `popover.rs`'s module docs describe)
//! neither stretches nor offsets it here either.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::dropdown;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "dropdown"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, dropdown::ID_ROOT, id)
}

/// **The wrap renders at all**, and it renders the shell rather than only the
/// trigger — the same first check `popover.rs`'s own test opens with, and for
/// the same reason: the popup is behind `if !open || !trigger_bounds_captured`,
/// true only from the second frame, which the shared `lay_out` harness
/// delivers.
#[gpui::test]
fn the_wrapped_shell_carries_its_root_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    assert!(seen.contains(&"dropdown-root".to_owned()), "{seen:?}");
    // Nothing from a neighbouring surface leaked in — two roots in one frame
    // would make `Snapshot::build` anchor to whichever it found first.
    assert!(!seen.iter().any(|id| id.starts_with("menu-")), "{seen:?}");
    assert!(
        !seen.iter().any(|id| id.starts_with("popover-")),
        "{seen:?}"
    );
    assert!(
        !seen.iter().any(|id| id.starts_with("git-row-")),
        "{seen:?}"
    );
}

/// **The shell is the declared width, to the pixel**, and sits at its own
/// origin by construction (§4).
///
/// `--shell-width`, not `--width` — the surface's own module docs record why:
/// `--width` is the shared, driver-owned surface width, and this primitive's
/// own quantity needed a name that does not collide with it.
#[gpui::test]
fn the_shell_is_the_declared_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [200u16, 240, 400] {
        let records = measure(cx, cell(&["--shell-width", &width.to_string()]));
        let root = at(&records, "dropdown-root");
        assert_px(root.size.width, px(f32::from(width)));
        assert_px(root.origin.x, px(0.0));
        assert_px(root.origin.y, px(0.0));
    }
}

/// **The root declares `content_sized` and says so**, the way `ANCHORS.md`
/// v1.5 requires: a declaration the differ trusts rather than infers. See
/// `crowbar_ui::components::dropdown::CONTENT_SIZED`'s doc comment for the
/// live measurement (`min-width: fit-content` beating a locked pixel width)
/// that overturned this module's first, wrong assumption.
#[gpui::test]
fn the_root_declares_content_sized_and_not_line_sized(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = find(&records, "dropdown-root");

    assert!(root.content_sized, "{root:?}");
    assert!(!root.line_sized, "{root:?}");
}

/// **The shell's height is two borders, two `p-1` paddings and the body** —
/// the arithmetic that makes `--body-height` a legitimate stand-in for a call
/// site's content rather than a fudge factor, the same property
/// `popover::Params::popup_height` establishes for its own two-box shell.
#[gpui::test]
fn the_shell_height_follows_the_body_through_its_border_and_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for body in [0u16, 120, 400] {
        let records = measure(cx, cell(&["--body-height", &body.to_string()]));
        let root = find(&records, "dropdown-root");
        let expected = f32::from(dropdown::BORDER_WIDTH) * 2.0
            + f32::from(dropdown::PADDING) * 2.0
            + f32::from(body);
        assert_px(root.bounds.size.height, px(expected));
    }
}

/// **The border is one real pixel**, the inverse of `dropdown-menu`'s ring
/// trap — `border.w` is compared *exactly*, so getting it the other way
/// round would fail every cell.
#[gpui::test]
fn the_shell_has_a_real_one_pixel_border_and_a_fourteen_pixel_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = find(&records, "dropdown-root");

    assert_px(root.border_width, px(1.0));
    assert_px(root.border_width, dropdown::BORDER_WIDTH);
    assert_px(root.radius, Theme::DARK.radius_xl.value());
    assert_px(root.radius, px(14.0));
    assert_eq!(
        root.background,
        Paint::Solid(
            Theme::DARK
                .card
                .mix(95.0, crowbar_ui::Color::TRANSPARENT)
                .value()
        ),
    );
    assert_eq!(root.border_color, Some(Theme::DARK.border.value()));
}

/// **`rounded-xl`/`bg-card` are `dropdown`'s own tokens, not `popover`'s** —
/// `popover-popup` is `rounded-lg` over `bg-popover` with no opacity mix, so a
/// port that reused those values here would be the `ANCHORS.md` v1.6 mistake
/// `components/mod.rs` warns about.
#[gpui::test]
fn the_shell_does_not_take_popovers_tokens(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = find(&records, "dropdown-root");

    assert_ne!(root.radius, Theme::DARK.radius_lg.value());
    assert_ne!(root.background, Paint::Solid(Theme::DARK.popover.value()),);
}

/// **`empty` collapses the body to zero** and moves the shell's height —
/// `provider-switch-dropdown.tsx`'s own no-other-provider branch is exactly
/// this shape.
#[gpui::test]
fn empty_collapses_the_body_to_a_bare_shell(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let root = find(&records, "dropdown-root");

    // 1 + 4 + 0 + 4 + 1.
    assert_px(root.bounds.size.height, px(10.0));
}

/// The light table paints a different shell, so a theme cell here is not
/// vacuous — `--card` and `--border` both move.
#[gpui::test]
fn the_light_table_paints_a_different_shell(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let root = find(&records, "dropdown-root");

    assert_eq!(
        root.background,
        Paint::Solid(
            Theme::LIGHT
                .card
                .mix(95.0, crowbar_ui::Color::TRANSPARENT)
                .value()
        ),
    );
    assert_eq!(root.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.card, Theme::DARK.card);
}
