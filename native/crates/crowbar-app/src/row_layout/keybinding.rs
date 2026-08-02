//! `--surface keybinding`, laid out in a real window.
//!
//! What this file has to establish that no other one does: that an **empty
//! legend records no anchor at all**. Every other surface in the tree asserts
//! that a box is where it should be; this one asserts, on one cell, that there
//! is no box — which is `ANCHORS.md` v1.11's ruling turned into a measurement
//! rather than a claim in a comment.
//!
//! The measurements the assertions are written against were taken off the
//! **running React app** at a 1714px viewport, dark, from the tab bar's
//! close-button tooltip (`button.tsx`'s `shortcut="mod+w"`), with the tooltip's
//! mount animation settled:
//!
//! ```text
//! keybinding  37.844 × 16   border 1px  radius 8  bg #1f1f1eff  fg #a4a4a4ff
//!             text "⌘W"     text_width 23.843     CalSansUI 12px/12px w400
//! ```
//!
//! # The reference had to be caught **at rest**, and v1.9 says why
//!
//! The first reading of that cap was `35.952 × 15.2`. Both numbers are exactly
//! 0.95 of the settled ones, because `tooltipContentBase` carries
//! `animate-in fade-in-0 zoom-in-95` and `WebKit`'s
//! `getBoundingClientRect()` returns the **transformed** box. A capture taken in
//! that window is indistinguishable from a port defect — the failure v1.9
//! describes, and the second time this port has hit it after `popover`'s 0.98.
//!
//! The settled numbers are the ones below, confirmed by re-reading after the
//! animation completed and cross-checked by arithmetic: `35.952 / 0.95` is
//! `37.844` and `15.2 / 0.95` is `16`.

use super::{a_cell, assert_px, find, ids, measure};
use crowbar_driver::Paint;
use crowbar_ui::Theme;
use crowbar_ui::components::keybinding;
use gpui::{TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "keybinding"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// **The surface carries exactly its one anchor**, and it is the root.
#[gpui::test]
fn the_surface_carries_exactly_its_one_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    assert_eq!(seen, vec!["keybinding".to_owned()], "{seen:?}");
}

/// **The border is one real pixel**, which is the inverse of `kbd`'s zero — and
/// `border.w` is the field `ANCHORS.md` v1.1 compares *exactly*, so getting it
/// the other way round fails every cell.
///
/// The radius is the theme's `--radius-md`, which this project redefines away
/// from Tailwind's stock 6 — and it is not `kbd`'s literal 4 either.
#[gpui::test]
fn the_cap_has_a_real_one_pixel_border_and_an_eight_pixel_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cap = find(&measure(cx, cell(&[])), "keybinding");

    assert_px(cap.border_width, px(1.0));
    assert_px(cap.border_width, keybinding::BORDER_WIDTH);
    assert_px(cap.radius, Theme::DARK.radius_md.value());
    assert_px(cap.radius, px(8.0));
    assert_eq!(cap.background, Paint::Solid(Theme::DARK.card.value()));
    assert_eq!(cap.border_color, Some(Theme::DARK.border.value()));

    // The control that makes the border assertion mean something: `scroll-area`,
    // ported in the same item, measures **0** on both of its boxes. A harness
    // that read a constant rather than the laid-out element would agree with
    // itself here whichever value the port carried.
    let area = find(
        &measure(cx, a_cell(&["--surface", "scroll-area"])),
        "scroll-area-root",
    );
    assert_px(area.border_width, px(0.0));
    assert!(
        (f32::from(cap.border_width) - f32::from(area.border_width)).abs() > 0.5,
        "the border trap runs both ways and these two are the shortest-range pair",
    );
}

/// **The box is the `min-h-4` floor, not the line box** — the declaration
/// `ANCHORS.md` v1.6 makes and the delta a wrong one would invent.
///
/// The height is 16 and the line box is 12; declaring `line_sized` here would
/// compare the first against the second on the surface's only anchor.
#[gpui::test]
fn the_cap_is_sixteen_tall_over_a_twelve_pixel_line_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cap = find(&measure(cx, cell(&[])), "keybinding");

    assert_px(cap.bounds.size.height, px(16.0));
    assert_px(cap.bounds.size.height, keybinding::MIN_HEIGHT);

    let text = cap.text.as_ref().expect("the cap paints its legend");
    assert_px(text.font.size, px(12.0));
    assert_px(text.font.line_height, px(12.0));
    assert!(
        (f32::from(cap.bounds.size.height) - f32::from(text.font.line_height)).abs() > 0.5,
        "a 4px gap is exactly the delta a wrong line_sized declaration invents: {cap:?}",
    );
}

/// **`content_sized` is declared and `line_sized` is not**, and both travel.
///
/// v1.5 and v1.6 make these properties the *component* asserts and the differ
/// trusts, so a declaration that silently stopped travelling would open a blind
/// spot and announce nothing.
#[gpui::test]
fn the_cap_declares_content_sized_and_never_line_sized(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cap = find(&measure(cx, cell(&[])), "keybinding");

    assert!(cap.content_sized, "{cap:?}");
    assert!(!cap.line_sized, "{cap:?}");
}

/// The legend travelled with the box: **what is recorded is what is painted**,
/// and on this component the string is the output of a parser rather than a
/// prop.
#[gpui::test]
fn the_recorded_legend_is_the_parsed_one(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cap = find(&measure(cx, cell(&[])), "keybinding");

    assert_eq!(
        cap.text.as_ref().map(|text| text.content.to_string()),
        Some("\u{2318}W".to_owned()),
    );
}

/// **The width is the legend's, with no floor under it** — which is what makes
/// `--content` a real axis here where `kbd`'s is half-bound by `min-w-5`.
///
/// Asserted as a strict ordering rather than three numbers: the port ceils a
/// text run's max-content width where `WebKit` keeps the fraction (v1.5), so the
/// absolute widths are the differ's business and the *ordering* is this
/// harness's.
#[gpui::test]
fn every_content_length_is_a_strictly_wider_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut previous = px(0.0);
    for length in ["short", "normal", "overflow"] {
        let cap = find(&measure(cx, cell(&["--content", length])), "keybinding");
        assert!(
            cap.bounds.size.width > previous,
            "{length} is not wider than the one before it: {cap:?}",
        );
        previous = cap.bounds.size.width;
        // And every one of them is taller than nothing and exactly the floor.
        assert_px(cap.bounds.size.height, keybinding::MIN_HEIGHT);
    }
}

/// The box is its legend's advance width plus `px-1.5` and two borders, with
/// **nothing else in it** — the arithmetic the reference's own numbers give:
/// `23.843 + 6 + 6 + 1 + 1 = 37.844`.
#[gpui::test]
fn the_box_is_the_advance_plus_its_padding_and_its_borders(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cap = find(&measure(cx, cell(&[])), "keybinding");
    let text = cap.text.as_ref().expect("the cap paints its legend");

    let chrome = 2.0 * f32::from(keybinding::PADDING_X) + 2.0 * f32::from(keybinding::BORDER_WIDTH);
    let expected = f32::from(text.width) + chrome;
    assert!(
        (f32::from(cap.bounds.size.width) - expected).abs() <= 1.0,
        "expected {expected} ± 1 (v1.5 ceils the run), got {:?}",
        cap.bounds.size.width,
    );
    // The chrome is 14px, which is the number a port that dropped the border
    // would get wrong by 2.
    assert!((chrome - 14.0).abs() < f32::EPSILON, "{chrome}");
}

/// **An empty legend records no anchor at all** — `ANCHORS.md` v1.11, and the
/// assertion this whole file exists for.
///
/// The port renders the flex row and nothing in it, so the registry comes back
/// empty and the binary refuses the cell for want of a root. That refusal is the
/// correct outcome: the reference emits nothing for the same cell, so the two
/// sides agree that there is nothing to compare.
///
/// A control follows it: the *same* cell without the flag records exactly one,
/// so this is a fact about the empty legend rather than about the harness.
#[gpui::test]
fn an_empty_legend_records_no_anchor_at_all(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec!["--flags", "empty"],
        vec!["--flags", "empty", "--keys", "Cmd,W"],
        vec!["--keys", ",,"],
    ] {
        let seen = ids(&measure(cx, cell(&args)));
        assert!(seen.is_empty(), "{args:?} recorded {seen:?}");
    }

    // The control.
    assert_eq!(ids(&measure(cx, cell(&[]))).len(), 1);
}

/// The platform branch reaches the **painted string**, not only the model: a
/// cell driven `other` records `Ctrl+W` and a wider box.
#[gpui::test]
fn the_other_platform_paints_a_different_and_wider_legend(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mac = find(&measure(cx, cell(&[])), "keybinding");
    let other = find(&measure(cx, cell(&["--platform", "other"])), "keybinding");

    assert_eq!(
        other.text.as_ref().map(|text| text.content.to_string()),
        Some("Ctrl+W".to_owned()),
    );
    assert_ne!(
        mac.text.as_ref().map(|t| t.content.to_string()),
        other.text.as_ref().map(|t| t.content.to_string()),
    );
    assert!(other.bounds.size.width > mac.bounds.size.width);
}

/// The light table paints a different cap, so a theme cell on this surface is
/// **not** vacuous — `--card`, `--border` and `--muted-foreground` all move.
///
/// Worth contrasting with `scroll-area` next door, where every field is
/// table-independent and the vacuity is asserted instead.
#[gpui::test]
fn the_light_table_paints_a_different_cap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cap = find(&measure(cx, cell(&["--theme", "light"])), "keybinding");

    assert_eq!(cap.background, Paint::Solid(Theme::LIGHT.card.value()));
    assert_eq!(cap.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.card, Theme::DARK.card);
    assert_ne!(Theme::LIGHT.border, Theme::DARK.border);
    assert_ne!(Theme::LIGHT.muted_foreground, Theme::DARK.muted_foreground);
}
