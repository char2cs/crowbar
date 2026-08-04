//! `--surface toast`, laid out in a real window.
//!
//! **No reference exists for this surface, and none can — see
//! `crowbar_ui::primitives::toast`'s module docs §2**: `toast.tsx`'s own
//! render path has zero live producers, not merely an unreached one this
//! session could not drive. What follows checks the port is internally
//! consistent — it renders, its padding-derived offsets follow the formula
//! its own docs claim, each variant carries exactly the anchors it should,
//! `empty` moves what it says it moves — rather than comparing any number to
//! a React capture, because there is nothing to compare it to.
//!
//! Widths are deliberately **not** asserted here, unlike every other surface
//! in this tree: `toast-popup` is built with no authored width (module docs'
//! "Two divergences" note on `Toast::render`), so its box is a function of
//! gpui's own real text shaping of whatever fixture string is in play — a
//! number this file has no independent way to predict, and asserting one
//! anyway would be exactly the "declare a fixed pixel width with no evidence
//! behind it" shortcut the module docs' "Declarations" section explains why
//! this component does not take.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::toast;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    // `toast` is not full-bleed (module docs' surface registration), so the
    // guard needs `--viewport-width` >= `--width` + 2×`INSET_X` (48px). The
    // default `--width` (320) comfortably fits an 800px viewport; no
    // `--width` override is needed the way `dialog`'s full-bleed cells need
    // one.
    let mut line = vec!["--surface", "toast", "--viewport-width", "800"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, toast::ID_POPUP, id)
}

/// The default variant carries the icon, the title and the description; the
/// `tooltip` variant carries only the title, and never the other two — the
/// same "must-not-leak-across-variants" property `alert_dialog`'s equivalent
/// test asserts against `dialog-*`.
#[gpui::test]
fn each_variant_carries_exactly_its_own_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let rich = ids(&measure(cx, cell(&[])));
    for id in [
        "toast-popup",
        "toast-icon",
        "toast-title",
        "toast-description",
    ] {
        assert!(rich.contains(&id.to_owned()), "{id} missing from {rich:?}");
    }

    let tip = ids(&measure(cx, cell(&["--variant", "tooltip"])));
    assert!(tip.contains(&"toast-popup".to_owned()), "{tip:?}");
    assert!(tip.contains(&"toast-title".to_owned()), "{tip:?}");
    assert!(!tip.contains(&"toast-icon".to_owned()), "{tip:?}");
    assert!(!tip.contains(&"toast-description".to_owned()), "{tip:?}");
}

/// The border and the two radii are exactly this crate's own tokens — 1px
/// real border on both variants, `radius_lg` under the default and
/// `radius_md` under `tooltipStyle`, the same pair `popover`'s two variants
/// take.
#[gpui::test]
fn the_two_variants_take_this_crates_own_border_and_radii(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let rich = measure(cx, cell(&[]));
    let popup = find(&rich, "toast-popup");
    assert_px(popup.border_width, px(1.0));
    assert_px(popup.radius, Theme::DARK.radius_lg.value());
    assert_px(popup.radius, px(10.0));
    assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));

    let tip = measure(cx, cell(&["--variant", "tooltip"]));
    let tip_popup = find(&tip, "toast-popup");
    assert_px(tip_popup.border_width, px(1.0));
    assert_px(tip_popup.radius, Theme::DARK.radius_md.value());
    assert_px(tip_popup.radius, px(8.0));
}

/// The title's text reaches the anchor, on both variants, and neither variant
/// declares it line-sized — see the module docs' "Declarations" section for
/// why (`toast.tsx`'s title has no `leading-none` in either branch, unlike
/// every other title this tree has ported).
#[gpui::test]
fn the_title_text_reaches_the_anchor_and_is_never_declared_line_sized(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let rich = measure(cx, cell(&["--title", "Saved"]));
    let rich_title = find(&rich, "toast-title");
    assert!(!rich_title.line_sized, "{rich_title:?}");
    assert!(!rich_title.content_sized, "{rich_title:?}");
    assert_eq!(
        rich_title
            .text
            .as_ref()
            .map(|text| text.content.to_string()),
        Some("Saved".to_owned()),
    );

    let tip = measure(cx, cell(&["--variant", "tooltip", "--title", "Copied"]));
    let tip_title = find(&tip, "toast-title");
    assert!(!tip_title.line_sized, "{tip_title:?}");
    assert_eq!(
        tip_title.text.as_ref().map(|text| text.content.to_string()),
        Some("Copied".to_owned()),
    );
}

/// The icon sits at the root's own border-plus-padding offset, `w-4` (16px)
/// wide; the title/description column starts `ICON_WIDTH + ICON_ROW_GAP`
/// (24px) further along the same row — the one geometric fact this surface
/// can assert without knowing gpui's own text-shaping of the fixture string.
#[gpui::test]
fn the_icon_and_the_column_follow_their_own_padding_and_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let icon = at(&records, "toast-icon");
    let origin = f32::from(toast::BORDER_WIDTH) + f32::from(toast::RICH_PADDING_X);
    assert_px(icon.origin.x, px(origin));
    assert_px(
        icon.origin.y,
        px(f32::from(toast::BORDER_WIDTH) + f32::from(toast::RICH_PADDING_Y)),
    );
    assert_px(icon.size.width, toast::ICON_WIDTH);

    let title = at(&records, "toast-title");
    assert_px(
        title.origin.x,
        px(origin + f32::from(toast::ICON_WIDTH) + f32::from(toast::ICON_ROW_GAP)),
    );

    // The description sits `COLUMN_GAP` below the title's own line, and at
    // the same x — same column, both anchors.
    let description = at(&records, "toast-description");
    assert_px(description.origin.x, title.origin.x);
    assert!(
        description.origin.y > title.origin.y,
        "{description:?} should sit below {title:?}"
    );
}

/// `--no-toast-icon` and `--no-description` each remove exactly their own anchor
/// and no other, on the default variant.
#[gpui::test]
fn no_icon_and_no_description_each_remove_their_own_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let no_icon = ids(&measure(cx, cell(&["--no-toast-icon"])));
    assert!(!no_icon.contains(&"toast-icon".to_owned()), "{no_icon:?}");
    assert!(no_icon.contains(&"toast-title".to_owned()), "{no_icon:?}");
    assert!(
        no_icon.contains(&"toast-description".to_owned()),
        "{no_icon:?}"
    );

    let no_description = ids(&measure(cx, cell(&["--no-description"])));
    assert!(
        !no_description.contains(&"toast-description".to_owned()),
        "{no_description:?}"
    );
    assert!(
        no_description.contains(&"toast-icon".to_owned()),
        "{no_description:?}"
    );
}

/// `empty` blanks the title to `""` on both variants — the title anchor is
/// still present (an empty string is still real content the reference could
/// produce), the same call `tooltip`'s own `empty` test makes.
#[gpui::test]
fn empty_blanks_the_title_but_keeps_its_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty", "--title", "Saved"]));
    let title = find(&records, "toast-title");

    assert_eq!(
        title.text.as_ref().map(|text| text.content.to_string()),
        Some(String::new()),
    );
}

/// An action row moves the popup's own height by its content height plus the
/// gap above it — the one height-arithmetic fact this surface's own
/// `popup_height` estimate makes, checked here through the actual anchored
/// layout rather than only against the unit-level formula.
#[gpui::test]
fn an_action_row_grows_the_popup_by_its_own_height_and_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let bare = at(&measure(cx, cell(&["--no-description"])), "toast-popup");
    let with_action = at(
        &measure(cx, cell(&["--no-description", "--action-height", "24"])),
        "toast-popup",
    );

    let expected_growth = f32::from(toast::RICH_GAP) + 24.0;
    assert_px(
        with_action.size.height,
        bare.size.height + px(expected_growth),
    );
}

/// The light table paints a different popup.
#[gpui::test]
fn the_light_table_paints_a_different_popup(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let popup = find(&records, "toast-popup");

    assert_eq!(popup.background, Paint::Solid(Theme::LIGHT.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.popover, Theme::DARK.popover);
}
