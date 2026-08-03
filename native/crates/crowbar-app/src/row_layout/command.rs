//! `--surface command`, laid out in a real window.
//!
//! Exactly `dialog`'s own file's reason for existing: `Command::render`
//! needs `window`/`cx` (`GpuiDialog::new` mints a `FocusHandle`), and this
//! confirms `render_ctx` carries the context through and the wrap lands
//! where its React original does — the same layers `dialog`'s file checks
//! (an outer box this crate does not hold, neutralised rather than switched
//! off; `w-full max-w-*` reproduced by hand against `window.viewport_size()`)
//! apply here unchanged, on a different set of numbers.
//!
//! The measurements below were taken off the **running React app** at a
//! 1714px viewport, dark, from the workspace switcher (Context Pill → click):
//!
//! ```text
//! popup                576 × 142   border 1px  radius 18  bg --popover
//! command (wrapper)    574 × 140
//! input row            574 × 48    (autocomplete-input-group 554 × 36)
//! command-panel        576 × 47    (-mx-px)  radius-t 14  border-b 0
//! command-item          558 × 30    data-highlighted
//! command-footer        574 × 45    border-t 1px
//! ```
//!
//! This surface needs the same two-frame delivery `popover`'s and `dialog`'s
//! do — the shared `lay_out` harness delivers it with nothing local to this
//! file.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::{autocomplete, command};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "command",
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
    relative_to(records, command::ID_POPUP, id)
}

/// **The wrap renders at all**, carrying every contract anchor the resting
/// cell has — its own three, and the shared `autocomplete-*`/`scroll-area-*`
/// ids the module docs say it reuses rather than re-derives.
#[gpui::test]
fn the_wrapped_popup_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        "command-dialog-popup",
        "command-panel",
        "command-footer",
        "autocomplete-input-group",
        "autocomplete-start-addon",
        "autocomplete-input",
        "autocomplete-empty",
        "autocomplete-item",
        "autocomplete-list",
        "scroll-area-root",
        "scroll-area-viewport",
    ] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    assert!(
        !seen.iter().any(|id| id.starts_with("dialog-")),
        "{seen:?}"
    );
    assert!(
        !seen.iter().any(|id| id.starts_with("popover-")),
        "{seen:?}"
    );
    // Exactly one workspace in the fixture: never two `autocomplete-item`s,
    // the property `ANCHORS.md` v1.8 needs.
    assert_eq!(
        seen.iter().filter(|id| *id == "autocomplete-item").count(),
        1,
        "{seen:?}"
    );
}

/// **The popup box is the reference's, to the pixel.**
#[gpui::test]
fn the_popup_is_the_measured_five_seventy_six_by_one_forty_two(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = at(&records, command::ID_POPUP);

    assert_px(popup.size.width, px(576.0));
    assert_px(popup.size.height, px(142.0));
    assert_px(popup.origin.x, px(0.0));
    assert_px(popup.origin.y, px(0.0));
}

/// **The border is one real pixel**, this crate's own theme.
#[gpui::test]
fn the_popup_has_this_crates_own_border_and_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = find(&records, command::ID_POPUP);

    assert_px(popup.border_width, px(1.0));
    assert_px(popup.border_width, command::BORDER_WIDTH);
    assert_px(popup.radius, Theme::DARK.radius_2xl.value());
    assert_px(popup.radius, px(18.0));
    assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));
}

/// The panel sits `-mx-px` outside the wrapper's own width and carries the
/// panel's own top radius and border colour.
#[gpui::test]
fn the_panel_is_the_measured_five_seventy_six_by_forty_seven(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let panel = at(&records, command::ID_PANEL);

    assert_px(panel.size.width, px(576.0));
    assert_px(panel.size.height, px(47.0));

    let panel_paint = find(&records, command::ID_PANEL);
    assert_px(panel_paint.border_width, command::BORDER_WIDTH);
    assert_px(panel_paint.radius, Theme::DARK.radius_xl.value());
    assert_px(panel_paint.radius, px(14.0));
}

/// The reachable item is `data-highlighted` by default (`autoHighlight`
/// parking the cursor on the sole row) and its box follows the panel's own
/// content width, one padding step inside the scroll area.
#[gpui::test]
fn the_item_is_highlighted_by_default_and_its_own_measured_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let item = at(&records, autocomplete::ID_ITEM);

    assert_px(item.size.width, px(558.0));
    assert_px(item.size.height, px(30.0));

    let painted = find(&records, autocomplete::ID_ITEM);
    assert_eq!(painted.background, Paint::Solid(Theme::DARK.accent.value()));
}

/// `--flags selected` is declared unmodelled (see
/// `crowbar_ui::components::command`'s surface module docs): `autoHighlight`
/// plus one row means the item is highlighted on every reachable cell, so
/// the flag renders byte-for-byte the same picture as the resting cell
/// rather than a second, unreached one.
///
/// **Compared `at` the popup root, not `find`'s window-absolute bounds.**
/// `GpuiDialog` (`gpui_component::dialog::Dialog`) plays an unconditional
/// 250ms slide-down + fade-in on every mount
/// (`vendor/gpui-component/src/dialog/dialog.rs`'s `ANIMATION_DURATION`,
/// `.with_animation("slide-down", …, |this, delta| this.top(y * delta) …)`)
/// — there is no field on the wrap that turns it off, unlike `.overlay`/
/// `.close_button`. `resting` and `flagged` are two *separate* windows, each
/// riding its own instance of that animation on its own wall-clock schedule,
/// so a `find`-style window-absolute comparison caught the two mid-slide at
/// different, independently-rounded offsets — flaky by real time elapsed
/// between `simulate_next_frame` calls, not by anything either cell renders
/// wrong (reproduced: identical flakiness measuring the same `cell(&[])`
/// twice in a row, `--flags` held constant). `at` subtracts the popup's own
/// origin first, and the item and the popup it is measured against share the
/// same animated ancestor, so the in-flight slide offset is on both sides of
/// the subtraction and cancels — exactly why every *other* assertion in this
/// file already measures `at` rather than `find`.
#[gpui::test]
fn selected_is_unmodelled_and_renders_the_same_picture(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let resting = measure(cx, cell(&[]));
    let flagged = measure(cx, cell(&["--flags", "selected"]));

    let resting_item = find(&resting, autocomplete::ID_ITEM);
    let flagged_item = find(&flagged, autocomplete::ID_ITEM);
    assert_eq!(resting_item.background, flagged_item.background);
    assert_eq!(resting_item.background, Paint::Solid(Theme::DARK.accent.value()));
    assert_eq!(
        at(&resting, autocomplete::ID_ITEM),
        at(&flagged, autocomplete::ID_ITEM),
    );
}

/// The footer sits flush against the panel's bottom border, its own height
/// following its content through its border and its padding.
#[gpui::test]
fn the_footer_height_follows_its_content(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for content in [20u16, 60, 100] {
        let records = measure(cx, cell(&["--footer-height", &content.to_string()]));
        let footer = at(&records, command::ID_FOOTER);
        let expected = f32::from(command::BORDER_WIDTH)
            + f32::from(command::FOOTER_PADDING_Y) * 2.0
            + f32::from(content);
        assert_px(footer.size.height, px(expected));
    }
}

/// `--max-width` moves the popup directly, up to the point the viewport
/// itself becomes the limit — `w-full max-w-xl`'s two arms, reproduced by
/// hand in `Command::render`.
#[gpui::test]
fn the_max_width_caps_the_popup_until_the_viewport_does(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [400u16, 576, 700] {
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
            at(&records, command::ID_POPUP).size.width,
            px(f32::from(width)),
        );
    }

    // Narrower than the cap: `w-full` wins, and the popup is the viewport
    // less the (unanchored) `CommandDialogViewport`'s own 16px padding
    // either side.
    let records = measure(
        cx,
        cell(&[
            "--max-width",
            "576",
            "--width",
            "300",
            "--viewport-width",
            "300",
        ]),
    );
    assert_px(at(&records, command::ID_POPUP).size.width, px(300.0 - 32.0));
}

/// `empty` swaps the row for the (unreached) empty state, dropping
/// `autocomplete-item` from the document and giving `autocomplete-empty` a
/// real, padded box.
#[gpui::test]
fn empty_swaps_the_row_for_the_empty_state(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let seen = ids(&records);

    assert!(
        !seen.contains(&"autocomplete-item".to_owned()),
        "{seen:?}"
    );
    let empty = at(&records, autocomplete::ID_EMPTY);
    assert!(f32::from(empty.size.height) > 0.0, "{empty:?}");
}

/// The light table paints a different popup.
#[gpui::test]
fn the_light_table_paints_a_different_popup(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let popup = find(&records, command::ID_POPUP);

    assert_eq!(popup.background, Paint::Solid(Theme::LIGHT.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.popover, Theme::DARK.popover);
}
