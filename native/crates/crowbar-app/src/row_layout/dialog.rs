//! `--surface dialog`, laid out in a real window.
//!
//! What this file has to establish that `popover`'s did not: that a wrapped
//! `gpui-component` widget whose constructor needs `window`/`cx` — see
//! `crowbar_ui::components::dialog`'s module docs for why `Dialog::new`
//! differs from `Popover::new` there — still lays out where its React
//! original does, once `render_ctx` carries the context through. The layers
//! `dialog` adds over `popover` — an outer box this crate does not hold,
//! neutralised in place rather than switched off, and `w-full max-w-*`
//! reproduced by hand against `window.viewport_size()` rather than left to
//! `Styled` — are exactly the kind of thing that could stretch or offset a
//! box without anyone noticing, which is what this file checks.
//!
//! The measurements the assertions are written against were taken off the
//! **running React app** at a 1714px viewport, dark, from
//! `add-repository-modal`'s `DialogContent className="sm:max-w-md"`:
//!
//! ```text
//! popup    448 × 307   border 1px  radius 18  bg --popover  16px/24px
//! header   446 × 68    padding 24 on all four sides (no description)
//! footer   446 × 65    border-top 1px  padding 16/24  bg --muted/72
//! ```
//!
//! This surface needs the same two-frame delivery `popover`'s does — `anchored()`
//! is `gpui`'s own deferred/anchored positioning primitive, and the shared
//! `lay_out` harness (`crowbar-driver`'s [`crowbar_driver::on_settled_frame`])
//! delivers it exactly as it does there, with nothing local to this file.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::dialog;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "dialog",
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
    relative_to(records, dialog::ID_POPUP, id)
}

/// **The wrap renders at all**, carrying every contract anchor the resting
/// cell has.
#[gpui::test]
fn the_wrapped_popup_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        "dialog-popup",
        "dialog-header",
        "dialog-title",
        "dialog-footer",
    ] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    // The reachable dialog renders no description, so that anchor must be
    // absent: one the reference cannot produce is a `FieldPresence` delta.
    assert!(!seen.contains(&"dialog-description".to_owned()), "{seen:?}");
    assert!(
        !seen.iter().any(|id| id.starts_with("popover-")),
        "{seen:?}"
    );
    assert!(
        !seen.iter().any(|id| id.starts_with("git-row-")),
        "{seen:?}"
    );
}

/// **The popup box is the reference's, to the pixel.** This is also the
/// assertion that caught the module's first draft: a `+ 2·BORDER_WIDTH`
/// compensation for the outer box's own border, written on the assumption
/// that `.border_0()` would zero the *colour* but not the *width* — measured
/// here at `450` against this `448`, which is what sent the module docs back
/// to `refine_style`'s real semantics.
#[gpui::test]
fn the_popup_is_the_measured_four_forty_eight_by_three_oh_seven(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = at(&records, "dialog-popup");

    assert_px(popup.size.width, px(448.0));
    assert_px(popup.size.height, px(307.0));
    assert_px(popup.origin.x, px(0.0));
    assert_px(popup.origin.y, px(0.0));
}

/// **The border is one real pixel**, and it is `theme.border`, not
/// `gpui-component`'s own theme — the whole point of neutralising the outer
/// box rather than leaving its defaults in place.
#[gpui::test]
fn the_popup_has_this_crates_own_border_and_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = find(&records, "dialog-popup");

    assert_px(popup.border_width, px(1.0));
    assert_px(popup.border_width, dialog::BORDER_WIDTH);
    assert_px(popup.radius, Theme::DARK.radius_2xl.value());
    assert_px(popup.radius, px(18.0));
    assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));
}

/// The header is the popup less its two borders on the width axis, and its
/// own content on the height axis: `24 + 20 + 24` with no description.
#[gpui::test]
fn the_header_is_the_popup_width_less_its_borders(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let header = at(&records, "dialog-header");

    assert_px(header.origin.x, dialog::BORDER_WIDTH);
    assert_px(header.origin.y, dialog::BORDER_WIDTH);
    assert_px(header.size.width, px(446.0));
    assert_px(header.size.height, px(68.0));
}

/// The title is its own line box and says so — `ANCHORS.md` v1.6 makes the
/// declaration a property the *component* asserts and the differ trusts.
#[gpui::test]
fn the_title_is_its_own_line_box_and_says_so(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let title = find(&records, "dialog-title");

    assert!(title.line_sized, "{title:?}");
    assert!(!title.content_sized, "{title:?}");
    assert_px(title.bounds.size.height, px(20.0));
    assert_eq!(
        title.text.as_ref().map(|text| text.content.to_string()),
        Some("Add repository".to_owned()),
    );
}

/// The footer sits flush against the popup's bottom border, its own height
/// following its content through its border and its padding.
#[gpui::test]
fn the_footer_height_follows_its_content(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for content in [32u16, 80, 120] {
        let records = measure(cx, cell(&["--footer-height", &content.to_string()]));
        let footer = at(&records, "dialog-footer");
        let expected = f32::from(dialog::BORDER_WIDTH)
            + f32::from(dialog::FOOTER_PADDING_Y) * 2.0
            + f32::from(content);
        assert_px(footer.size.height, px(expected));

        let popup = at(&records, "dialog-popup");
        // Two borders, the fixed 68px header, the fixed 172px body, and the
        // footer that just moved.
        assert_px(popup.size.height, px(2.0 + 68.0 + 172.0 + expected));
    }
}

/// `--max-width` moves the popup directly, up to the point the viewport
/// itself becomes the limit — `w-full max-w-*`'s two arms, reproduced by
/// hand in `Dialog::render`.
#[gpui::test]
fn the_max_width_caps_the_popup_until_the_viewport_does(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [300u16, 448, 600] {
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
            at(&records, "dialog-popup").size.width,
            px(f32::from(width)),
        );
    }

    // Narrower than the cap: `w-full` wins, and the popup is the viewport
    // less the (unanchored) `DialogViewport`'s own 16px padding either side.
    let records = measure(
        cx,
        cell(&[
            "--max-width",
            "448",
            "--width",
            "300",
            "--viewport-width",
            "300",
        ]),
    );
    assert_px(at(&records, "dialog-popup").size.width, px(300.0 - 32.0));
}

/// `empty` removes the header and the footer together, and the popup
/// shrinks to its two borders around the body.
#[gpui::test]
fn empty_removes_the_header_and_the_footer(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let seen = ids(&records);

    assert!(!seen.contains(&"dialog-header".to_owned()), "{seen:?}");
    assert!(!seen.contains(&"dialog-footer".to_owned()), "{seen:?}");
    let popup = at(&records, "dialog-popup");
    assert_px(popup.size.height, px(2.0 + 172.0));
}

/// The light table paints a different popup.
#[gpui::test]
fn the_light_table_paints_a_different_popup(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let popup = find(&records, "dialog-popup");

    assert_eq!(popup.background, Paint::Solid(Theme::LIGHT.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.popover, Theme::DARK.popover);
}
