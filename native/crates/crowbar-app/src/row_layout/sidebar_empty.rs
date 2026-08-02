//! `--surface sidebar-empty`, laid out in a real window.
//!
//! The measurements the assertions are written against were taken off the
//! **running React app** at a 1714px viewport, dark, with the file explorer's
//! tree filter holding a string that matches nothing:
//!
//! ```text
//! sidebar-empty            123.94 × 96   at (0, 0)   bg rgba(0,0,0,0)  radius 0  border 0
//! sidebar-empty-message     99.94 × 16   at (12, 40) fg #a4a4a4ff  "No matching files"
//!                                        font CalSansUI 12 / 16.2, weight 400
//! ```
//!
//! Three numbers in there are the whole surface and each one is a different
//! mechanism:
//!
//! * **96** is `min-h-24` beating a 64.2px column — the one place this component
//!   has a size of its own rather than its content's.
//! * **40** is `justify-center` putting a 16px line in the 48px the padding
//!   leaves: 24 + (48 − 16) / 2.
//! * **123.94 / 99.94** are one text run's max-content width, once with 24px of
//!   `px-3` around it. Both are declared `content_sized`, and `ceil` carries
//!   through integral padding — `ceil(99.94) + 24 == ceil(123.94)` — which is
//!   what makes declaring the *container* alongside the run correct rather than
//!   convenient.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::sidebar;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "sidebar-empty"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, sidebar::ID_EMPTY, id)
}

/// The reachable cell carries exactly the two anchors the reference has, and no
/// more: the icon and the description are behind props no live call site passes,
/// and an anchor the reference cannot produce is a `FieldPresence` delta.
#[gpui::test]
fn the_reachable_cell_carries_the_references_two_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--width", "344"])));

    assert!(seen.contains(&"sidebar-empty".to_owned()), "{seen:?}");
    assert!(
        seen.contains(&"sidebar-empty-message".to_owned()),
        "{seen:?}"
    );
    assert!(!seen.contains(&"sidebar-empty-icon".to_owned()), "{seen:?}");
    assert!(
        !seen.contains(&"sidebar-empty-description".to_owned()),
        "{seen:?}"
    );
    assert_eq!(seen.len(), 2, "{seen:?}");
}

/// **`min-h-24` is what decides the height**, and this is the assertion that
/// says so rather than assuming it: the column inside is 64.2px, so a port that
/// dropped the minimum would draw a 65px box and move every bound under it.
#[gpui::test]
fn the_box_is_the_minimum_rather_than_its_content(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344"]));
    let empty = at(&records, "sidebar-empty");

    assert_px(empty.size.height, px(96.0));
    assert_px(empty.origin.x, px(0.0));
    assert_px(empty.origin.y, px(0.0));

    // The column it beat: 24 + 16.2 + 24.
    let column = f32::from(sidebar::EMPTY_PADDING_Y) * 2.0 + 12.0 * sidebar::EMPTY_LINE_HEIGHT;
    assert!((column - 64.2).abs() < 0.01, "{column}");
    assert!(column < f32::from(sidebar::EMPTY_MIN_HEIGHT));
}

/// **The message sits at (12, 40)**, which is `px-3` on one axis and
/// `justify-center` on the other — the two numbers a reader can check against
/// the reference by hand.
#[gpui::test]
fn the_message_is_centred_inside_the_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344"]));
    let message = at(&records, "sidebar-empty-message");

    assert_px(message.origin.x, sidebar::EMPTY_PADDING_X);
    assert_px(message.origin.x, px(12.0));
    assert_px(message.origin.y, px(40.0));

    // 24 + (96 − 48 − 16) / 2, spelled out.
    let free = 96.0 - f32::from(sidebar::EMPTY_PADDING_Y) * 2.0 - 16.0;
    assert!((f32::from(sidebar::EMPTY_PADDING_Y) + free / 2.0 - 40.0).abs() < 0.01);
}

/// **The message's box is its own line box**, and it says so — `ANCHORS.md` v1.6
/// makes the declaration a property the *component* asserts and the differ
/// trusts, so a declaration that silently stopped travelling would open a blind
/// spot and announce nothing.
///
/// The two engines' numbers are 16 (`WebKit`'s floor) against 16.0 (gpui's
/// `pixel_snap`) of the same 16.2, which is why the rule compares the native box
/// against the reference's `font.line_height` rather than its `bounds.h`.
#[gpui::test]
fn the_message_is_its_own_line_box_and_says_so(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344"]));
    let message = find(&records, "sidebar-empty-message");

    assert!(message.line_sized, "{message:?}");
    assert!(message.content_sized, "{message:?}");
    assert_px(message.bounds.size.height, px(16.0));

    let text = message.text.as_ref().expect("the message paints a run");
    assert_eq!(text.content.to_string(), "No matching files");
    assert!((f32::from(text.font.size) - 12.0).abs() < 0.01, "{text:?}");
    // gpui reports the line height it *snapped*, where the reference reports the
    // fractional 16.2 the stylesheet asked for. v1.6 compares the native box
    // against that 16.2 at the ordinary ±0.5, which is the slack this asserts.
    assert!(
        (f32::from(text.font.line_height) - 16.2).abs() <= 0.5,
        "{text:?}"
    );
    assert_px(message.bounds.size.height, text.font.line_height);
}

/// **The container is declared `content_sized` and the arithmetic that justifies
/// it holds in a real layout**: the box is the run plus 24px, at every string.
///
/// This is the assertion that would catch the container being stretched — by the
/// call-site wrapper losing its `items_center`, or by the surface dropping the
/// row flex altogether — in which case its width would be `--width` and the
/// declaration would be a lie about a box nothing shrink-wrapped.
#[gpui::test]
fn the_container_is_the_run_plus_two_paddings(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for message in ["No matching files", "Folder is empty", "Searching files"] {
        let records = measure(cx, cell(&["--width", "344", "--message", message]));
        let empty = at(&records, "sidebar-empty");
        let run = at(&records, "sidebar-empty-message");

        assert_px(
            empty.size.width,
            run.size.width + sidebar::EMPTY_PADDING_X * 2.0,
        );
        assert!(f32::from(empty.size.width) < 344.0, "{message}: stretched");
        assert!(find(&records, "sidebar-empty").content_sized);
    }
}

/// The icon adds its own row **and** the gap and its `mb-0.5`, and only past 96
/// does the box grow at all — which is `min-h-24` doing its job on the native
/// side too.
#[gpui::test]
fn the_icon_adds_a_row_a_gap_and_its_margin(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344", "--icon"]));
    let empty = at(&records, "sidebar-empty");
    let icon = at(&records, "sidebar-empty-icon");

    assert_px(icon.size.width, sidebar::EMPTY_ICON_EXTENT);
    assert_px(icon.size.height, sidebar::EMPTY_ICON_EXTENT);

    // The message follows it by the gap plus the icon's own bottom margin.
    let message = at(&records, "sidebar-empty-message");
    assert_px(
        message.origin.y,
        icon.origin.y + icon.size.height + sidebar::EMPTY_ICON_MARGIN_BOTTOM + sidebar::EMPTY_GAP,
    );

    // 24 + 28 + 2 + 6 + the line box + 24, which is past the 96 minimum — so
    // the box is its content here and `min-h-24` is no longer what decides it.
    // Written out of the boxes rather than out of 100.2, because the line box
    // is the one term this harness quantises: gpui snaps 16.2 to the device
    // pixel grid and a literal here would be asserting the DPR.
    let expected = f32::from(sidebar::EMPTY_PADDING_Y) * 2.0
        + f32::from(icon.size.height)
        + f32::from(sidebar::EMPTY_ICON_MARGIN_BOTTOM)
        + f32::from(sidebar::EMPTY_GAP)
        + f32::from(message.size.height);
    assert_px(empty.size.height, px(expected));
    assert!(
        expected > f32::from(sidebar::EMPTY_MIN_HEIGHT),
        "{expected}"
    );
}

/// The description is a **smaller** run on the same 1.35 line, and it is
/// declared neither `content_sized` nor `line_sized` — `max-w-[24ch]` makes it a
/// line box only while the string fits on one line, and the component cannot
/// know that.
#[gpui::test]
fn the_description_is_a_smaller_run_and_declares_nothing(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(
        cx,
        cell(&["--width", "344", "--description", "Try another search."]),
    );
    let description = find(&records, "sidebar-empty-description");
    let message = find(&records, "sidebar-empty-message");

    assert!(!description.line_sized, "{description:?}");
    assert!(!description.content_sized, "{description:?}");

    let small = description.text.as_ref().expect("a run");
    let big = message.text.as_ref().expect("a run");
    assert!(
        (f32::from(small.font.size) - 11.0).abs() < 0.01,
        "{small:?}"
    );
    assert!(f32::from(small.font.size) < f32::from(big.font.size));
}

/// The three tones paint three different foregrounds on the message, so a tone
/// cell is a different picture rather than a second spelling of the same one.
#[gpui::test]
fn the_tones_repaint_the_message(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let neutral = find(
        &measure(cx, cell(&["--width", "344"])),
        "sidebar-empty-message",
    );
    let error = find(
        &measure(cx, cell(&["--width", "344", "--tone", "error"])),
        "sidebar-empty-message",
    );
    let success = find(
        &measure(cx, cell(&["--width", "344", "--tone", "success"])),
        "sidebar-empty-message",
    );

    let colour = |anchor: &crowbar_driver::RawAnchor| anchor.text.as_ref().expect("a run").color;
    assert_eq!(colour(&neutral), Theme::DARK.muted_foreground.value());
    assert_eq!(colour(&error), Theme::DARK.destructive.value());
    assert_eq!(colour(&success), Theme::DARK.success.value());
    assert_ne!(colour(&neutral), colour(&error));
}

/// The light table paints a different column, so a theme cell on **this**
/// surface is not vacuous — unlike `sidebar-header`, whose box takes no token
/// at all.
#[gpui::test]
fn the_light_table_paints_a_different_message(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344", "--theme", "light"]));
    let message = find(&records, "sidebar-empty-message");

    assert_eq!(
        message.text.as_ref().expect("a run").color,
        Theme::LIGHT.muted_foreground.value(),
    );
    assert_ne!(Theme::LIGHT.muted_foreground, Theme::DARK.muted_foreground);
    // …and the box itself still paints nothing on either table.
    let empty = find(&records, "sidebar-empty");
    assert_eq!(empty.background, Paint::None);
    assert_px(empty.radius, px(0.0));
    assert_px(empty.border_width, px(0.0));
}
