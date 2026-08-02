//! `--surface sheet`, laid out in a real window.
//!
//! **No reference exists for this surface** — see
//! `crowbar_ui::components::sheet`'s module docs. What follows checks the
//! port is internally consistent (it renders, its width follows the formula
//! its own docs claim, `empty` moves what it says it moves) rather than
//! comparing any number to a React capture, because there is no capture to
//! compare it to.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_ui::Theme;
use crowbar_ui::components::sheet;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "sheet",
        "--width",
        "1714",
        "--viewport-width",
        "1714",
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

fn at(records: &[crowbar_driver::RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, sheet::ID_POPUP, id)
}

/// The wrap renders, carrying every contract anchor the resting cell has.
#[gpui::test]
fn the_wrapped_panel_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in ["sheet-popup", "sheet-header", "sheet-title"] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    assert!(!seen.contains(&"sheet-description".to_owned()), "{seen:?}");
    assert!(!seen.iter().any(|id| id.starts_with("dialog-")), "{seen:?}");
}

/// The panel's main axis follows `w-[calc(100%-(--spacing(12)))] max-w-md`,
/// the same two-armed formula `dialog`'s `max_width` uses.
#[gpui::test]
fn the_panel_width_follows_the_viewport_until_max_size_caps_it(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    // Capped: 1714 − 48 exceeds MAX_SIZE (448), so 448 wins.
    let wide = measure(cx, cell(&[]));
    assert_px(at(&wide, "sheet-popup").size.width, px(448.0));

    // Uncapped: a narrow window makes `w-[calc(100%-48px)]` the binding arm.
    let narrow = measure(cx, cell(&["--width", "300", "--viewport-width", "300"]));
    assert_px(at(&narrow, "sheet-popup").size.width, px(300.0 - 48.0));
}

/// The border is one real pixel and `theme.border`'s colour, whatever the
/// vendor's own un-overridable width happens to agree with — see the module
/// docs' point 1.
#[gpui::test]
fn the_panel_has_a_one_pixel_border_in_this_crates_colour(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = find(&records, "sheet-popup");

    assert_px(popup.border_width, px(1.0));
    assert_px(popup.border_width, sheet::BORDER_WIDTH);
    assert_eq!(
        popup.background,
        crowbar_driver::Paint::Solid(Theme::DARK.popover.value())
    );
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));
}

/// The title is its own line box and says so.
#[gpui::test]
fn the_title_is_its_own_line_box_and_says_so(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let title = find(&records, "sheet-title");

    assert!(title.line_sized, "{title:?}");
    assert!(!title.content_sized, "{title:?}");
    assert_px(title.bounds.size.height, px(20.0));
    assert_eq!(
        title.text.as_ref().map(|text| text.content.to_string()),
        Some("Sidebar".to_owned()),
    );
}

/// `empty` removes the header.
#[gpui::test]
fn empty_removes_the_header(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let seen = ids(&records);

    assert!(!seen.contains(&"sheet-header".to_owned()), "{seen:?}");
    assert!(!seen.contains(&"sheet-title".to_owned()), "{seen:?}");
}
