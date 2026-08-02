//! `--surface alert-dialog`, laid out in a real window.
//!
//! **No live pixel reference exists for this surface** — see
//! `crowbar_ui::components::alert_dialog`'s module docs §3 for what was
//! actually tried (a real review thread opened over the daemon's own REST API,
//! a route navigated to directly) and for the environmental `IndexedDB` defect
//! that blocked it, unrelated to this item. What follows checks two things
//! instead: that the wrap renders and lays out **exactly like `dialog`'s
//! already-converged wrap does** on every field the two share (which is the
//! evidence this item has in place of a second live capture — see the module
//! docs' class-list diff), and that the port is internally consistent on the
//! one field that differs for a real reason (`body_height` = 0).

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::alert_dialog;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "alert-dialog",
        "--width",
        "1714",
        "--viewport-width",
        "1714",
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, alert_dialog::ID_POPUP, id)
}

/// **The wrap renders at all**, carrying every contract anchor the resting
/// cell has.
#[gpui::test]
fn the_wrapped_popup_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        "alert-dialog-popup",
        "alert-dialog-header",
        "alert-dialog-title",
        "alert-dialog-description",
        "alert-dialog-footer",
    ] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    assert!(
        !seen.iter().any(|id| id.starts_with("dialog-")),
        "{seen:?} — this surface must never carry a bare `dialog-*` id, even \
         though the two components' constants are equal"
    );
}

/// The popup's own box, at the fixture's real dimensions — 512 wide
/// (`max-w-lg`, uncapped at a 1714px viewport) by 159 tall (two borders, the
/// header, a zero-height body, the footer).
#[gpui::test]
fn the_popup_is_five_twelve_by_one_fifty_nine(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = at(&records, "alert-dialog-popup");

    assert_px(popup.size.width, px(512.0));
    assert_px(popup.size.height, px(159.0));
    assert_px(popup.origin.x, px(0.0));
    assert_px(popup.origin.y, px(0.0));
}

/// The border and the radius are `dialog`'s own values, to the pixel — the
/// class lists are identical on this field (module docs' table), so this is
/// the one number the class diff alone already guarantees.
#[gpui::test]
fn the_popup_has_this_crates_own_border_and_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = find(&records, "alert-dialog-popup");

    assert_px(popup.border_width, px(1.0));
    assert_px(popup.border_width, alert_dialog::BORDER_WIDTH);
    assert_px(popup.radius, Theme::DARK.radius_2xl.value());
    assert_px(popup.radius, px(18.0));
    assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));
}

/// The header is the popup less its two borders on the width axis, and its
/// own content on the height axis: `24 + 20 + 8 + 20 + 24` — a title **and** a
/// description, unlike `dialog`'s own reachable reference (title only).
#[gpui::test]
fn the_header_carries_both_the_title_and_the_description(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let header = at(&records, "alert-dialog-header");

    assert_px(header.origin.x, alert_dialog::BORDER_WIDTH);
    assert_px(header.origin.y, alert_dialog::BORDER_WIDTH);
    assert_px(header.size.width, px(510.0));
    assert_px(header.size.height, px(96.0));
}

/// The title is its own line box and says so.
#[gpui::test]
fn the_title_is_its_own_line_box_and_says_so(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let title = find(&records, "alert-dialog-title");

    assert!(title.line_sized, "{title:?}");
    assert!(!title.content_sized, "{title:?}");
    assert_px(title.bounds.size.height, px(20.0));
    assert_eq!(
        title.text.as_ref().map(|text| text.content.to_string()),
        Some("Delete this thread?".to_owned()),
    );
}

/// The description is real and modelled, unlike `dialog`'s own reachable
/// reference — see the module docs' fixture account.
#[gpui::test]
fn the_description_is_modelled_and_reached(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let description = find(&records, "alert-dialog-description");

    assert!(!description.line_sized, "{description:?}");
    assert_eq!(
        description
            .text
            .as_ref()
            .map(|text| text.content.to_string()),
        Some("This permanently deletes the comment and its thread.".to_owned()),
    );
}

/// The footer sits flush against the popup's bottom border, its own height
/// following its content through its border and its padding — the same
/// arithmetic `dialog`'s footer test holds, at this surface's own default
/// (`size="sm"`) 28px content height rather than `dialog`'s 32px.
#[gpui::test]
fn the_footer_height_follows_its_content(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for content in [28u16, 60, 100] {
        let records = measure(cx, cell(&["--footer-height", &content.to_string()]));
        let footer = at(&records, "alert-dialog-footer");
        let expected = f32::from(alert_dialog::BORDER_WIDTH)
            + f32::from(alert_dialog::FOOTER_PADDING_Y) * 2.0
            + f32::from(content);
        assert_px(footer.size.height, px(expected));

        let popup = at(&records, "alert-dialog-popup");
        // Two borders, the fixed 96px header, a zero-height body, and the
        // footer that just moved.
        assert_px(popup.size.height, px(2.0 + 96.0 + 0.0 + expected));
    }
}

/// `--max-width` moves the popup directly, up to the point the viewport
/// itself becomes the limit — the identical two-armed formula `dialog`'s
/// `max_width` uses.
#[gpui::test]
fn the_max_width_caps_the_popup_until_the_viewport_does(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [300u16, 512, 600] {
        let records = measure(
            cx,
            cell(&[
                "--max-width",
                &width.to_string(),
                "--width",
                "1200",
                "--viewport-width",
                "1200",
            ]),
        );
        assert_px(
            at(&records, "alert-dialog-popup").size.width,
            px(f32::from(width)),
        );
    }

    // Narrower than the cap: `w-full` wins, and the popup is the viewport
    // less the (unanchored) `AlertDialogViewport`'s own 16px padding either
    // side.
    let records = measure(
        cx,
        cell(&[
            "--max-width",
            "512",
            "--width",
            "300",
            "--viewport-width",
            "300",
        ]),
    );
    assert_px(
        at(&records, "alert-dialog-popup").size.width,
        px(300.0 - 32.0),
    );
}

/// `empty` removes the header and the footer together, and the popup shrinks
/// to its two borders around a zero-height body — the identical arithmetic
/// `dialog`'s `empty` test holds, minus the body's own 172px because this
/// surface's real body is zero either way.
#[gpui::test]
fn empty_removes_the_header_and_the_footer(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let seen = ids(&records);

    assert!(
        !seen.contains(&"alert-dialog-header".to_owned()),
        "{seen:?}"
    );
    assert!(
        !seen.contains(&"alert-dialog-footer".to_owned()),
        "{seen:?}"
    );
    let popup = at(&records, "alert-dialog-popup");
    assert_px(popup.size.height, px(2.0));
}

/// The light table paints a different popup.
#[gpui::test]
fn the_light_table_paints_a_different_popup(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let popup = find(&records, "alert-dialog-popup");

    assert_eq!(popup.background, Paint::Solid(Theme::LIGHT.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.popover, Theme::DARK.popover);
}
