//! `--surface search`, laid out in a real window, against
//! `/tmp/p3-ref-search.json` (root `search-popover`, replace collapsed) and
//! `/tmp/p3-ref-search-replace-row.json` (root `search-replace-row`, replace
//! expanded — measured separately for the reason `crowbar_ui::components::
//! search`'s module docs give).
//!
//! Nothing here is a wrapped `gpui-component` popup — `search` is "built, not
//! wrapped" (the module docs' seam test) — so this surface renders in the
//! **first** draw, the same shape `button`'s and `dropdown_menu`'s own
//! `row_layout` files establish. What is worth a dedicated test here, on top
//! of that: the size-6/`sm:h-8` trap actually moves the rendered box at the
//! breakpoint, and the two labelled replace buttons actually come out
//! content-sized.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::search;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "search"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// A cell rooted at the replace row — `--surface search --replace`, but read
/// back relative to `search-replace-row` rather than `search-popover`. The
/// binary's own `--surface search --replace` still renders both roots in one
/// tree; this harness just measures the inner one directly, exactly as the
/// live capture that produced `/tmp/p3-ref-search-replace-row.json` did.
fn replace_cell(args: &[&str]) -> Cell {
    let mut line = vec!["--replace"];
    line.extend_from_slice(args);
    cell(&line)
}

/// Bounds relative to `search-popover`.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, search::ID_POPOVER, id)
}

/// Bounds relative to `search-replace-row`.
fn at_replace_row(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, search::ID_REPLACE_ROW, id)
}

/// **The shell renders at all**, at the declared width, and at its own
/// origin (§4).
#[gpui::test]
fn the_popover_carries_its_root_anchor_at_the_declared_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let root = at(&records, "search-popover");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(320.0));

    let seen = ids(&records);
    assert!(seen.contains(&"search-popover".to_owned()), "{seen:?}");
}

/// **The default cell reproduces `/tmp/p3-ref-search.json`'s own root box**:
/// `320×84`, `radius 14`, `border.w 1`.
#[gpui::test]
fn the_default_cells_root_matches_the_captured_reference(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = find(&records, "search-popover");

    assert_px(root.bounds.size.height, px(84.0));
    assert_px(root.radius, px(14.0));
    assert_px(root.border_width, px(1.0));
    assert_eq!(
        root.background,
        Paint::Solid(
            Theme::DARK
                .background
                .mix(95.0, crowbar_ui::Color::TRANSPARENT)
                .value()
        ),
    );
}

/// **The size-6/`sm:h-8` trap actually moves the rendered box.** At `Sm` the
/// leading toggle is `24×32` — `button`'s own `sm:h-8` surviving the merge;
/// below it, nothing competes with `size-6` and the box is a bare `24×24`.
/// Both numbers were confirmed live, not reasoned through in one direction
/// only (`native/mapping/search.md` §2).
#[gpui::test]
fn the_icon_buttons_size_moves_at_the_sm_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "1714"]));
    let wide_toggle = at(&wide, "search-replace-toggle");
    assert_px(wide_toggle.size.width, px(24.0));
    assert_px(wide_toggle.size.height, px(32.0));

    let narrow = measure(cx, cell(&["--viewport-width", "500"]));
    let narrow_toggle = at(&narrow, "search-replace-toggle");
    assert_px(narrow_toggle.size.width, px(24.0));
    assert_px(narrow_toggle.size.height, px(24.0));
}

/// Every icon-shaped anchor on the default cell shares the same `24×32` box
/// — the close button and both nav chevrons included, not only the leading
/// toggle.
#[gpui::test]
fn every_icon_button_on_the_default_cell_is_the_same_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for id in [
        "search-replace-toggle",
        "search-close",
        "search-toggle-case-sensitive",
        "search-toggle-whole-word",
        "search-toggle-regex",
        "search-nav-previous",
        "search-nav-next",
    ] {
        let bounds = at(&records, id);
        assert_px(bounds.size.width, px(24.0));
        assert_px(bounds.size.height, px(32.0));
    }
}

/// **The input control is `246×32`** — `search.tsx`'s own `h-8` override on
/// the outer span, decoupled from the inner field's `input`-primitive
/// `Size::Default` height (`30` at `Sm`). Both numbers match
/// `/tmp/p3-ref-search.json` exactly.
#[gpui::test]
fn the_input_control_and_field_keep_their_measured_split_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = at(&records, "input-control");
    assert_px(control.size.width, px(246.0));
    assert_px(control.size.height, px(32.0));

    let field = at(&records, "input");
    assert_px(field.size.height, px(30.0));
    assert_ne!(
        control.size.height, field.size.height,
        "the control's own h-8 override and the field's unmodified Size::Default height differ",
    );
}

/// **Defect 1 (P3.44).** `search.tsx`'s own `pl-8 pr-8 py-1` on
/// `SearchPopover`'s `Input` insets the field from its control by 33px
/// horizontally (the control's constant 1px border plus `pl-8`/`pr-8`'s
/// 32px) and 5px vertically (the same border plus `py-1`'s 4px) —
/// `/tmp/p3-ref-search.json`'s own `input-control` `37,7,246,32` against
/// `input` `70,12,180,30`. Expressed as the relationship the reference
/// implies (an inset from the control's own edges), not as bare absolute
/// coordinates, so it keeps meaning if the popover's width ever moves.
///
/// Turns red on the one-line revert `crates/crowbar-ui/src/components/
/// search.rs`'s `input_control` from `.items_start()` back to
/// `.items_center()` (the vertical assertions) or from `.pl(padding_x)`
/// back to a shared `.px(px(11.0))` (the horizontal ones) — see this test's
/// own mutation evidence in the P3.44 report.
#[gpui::test]
fn the_input_field_insets_from_its_control_by_the_icon_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = at(&records, "input-control");
    let field = at(&records, "input");

    let left_inset = field.origin.x - control.origin.x;
    let right_inset = (control.origin.x + control.size.width) - (field.origin.x + field.size.width);
    let top_inset = field.origin.y - control.origin.y;

    assert_px(left_inset, px(33.0));
    assert_px(right_inset, px(33.0));
    assert_px(top_inset, px(5.0));
}

/// **Defect 3 (P3.44).** `search-toggle-icon` is `search-toggle-icons`' own
/// anchor (P3.8, a separate, already-verified surface), reused unmodified
/// inside every toggle button here — but it must not be recorded a second
/// time on *this* surface's own registry, which is exactly what
/// `web/src/lib/oracle/extract.ts`'s `oracleSurfaceScope` already enforces
/// on the reference side (`search`'s declared ten ids do not include it).
///
/// Turns red on the one-line revert in `crates/crowbar-ui/src/components/
/// search.rs`'s `toggle_button`, from `icon.render(theme, &Unanchored)`
/// back to `icon.render(theme, anchors)` — see the P3.44 report's mutation
/// evidence.
#[gpui::test]
fn the_toggle_icon_is_not_recorded_on_this_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--replace"]));

    let seen = ids(&records);
    assert!(
        !seen.contains(&"search-toggle-icon".to_owned()),
        "search-toggle-icon belongs to the search-toggle-icons surface, not this one: {seen:?}",
    );
}

/// `--can-navigate` moves the two nav chevrons off `DISABLED_OPACITY` — the
/// field this surface's caption calls out, and the only visible effect
/// `canNavigate` has (their box does not move).
#[gpui::test]
fn can_navigate_moves_the_nav_chevrons_off_disabled_opacity(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let disabled = measure(cx, cell(&[]));
    let enabled = measure(cx, cell(&["--can-navigate"]));

    // The box does not move either way — only opacity, which the contract
    // cannot see (`ANCHORS.md` §6) but is still worth confirming stays put.
    assert_px(
        at(&disabled, "search-nav-previous").size.width,
        at(&enabled, "search-nav-previous").size.width,
    );
    assert_eq!(
        find(&disabled, "search-nav-previous").background,
        find(&enabled, "search-nav-previous").background,
    );
}

/// **`--replace` mounts the fourth toggle and the replace row together.**
#[gpui::test]
fn replace_mounts_the_fourth_toggle(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let collapsed = ids(&measure(cx, cell(&[])));
    assert!(!collapsed.contains(&"search-toggle-preserve-case".to_owned()));

    let expanded = ids(&measure(cx, cell(&["--replace"])));
    assert!(expanded.contains(&"search-toggle-preserve-case".to_owned()));
    assert!(expanded.contains(&"search-replace-row".to_owned()));
}

/// **The replace row's own subtree matches `/tmp/p3-ref-search-replace-row.json`**:
/// the swap icon at `32×32`, and the two labelled buttons at their measured,
/// content-sized widths.
#[gpui::test]
fn the_replace_row_matches_the_captured_reference(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, replace_cell(&[]));

    let row = at_replace_row(&records, "search-replace-row");
    assert_px(row.origin.x, px(0.0));
    assert_px(row.origin.y, px(0.0));

    let icon = at_replace_row(&records, "search-replace-icon");
    assert_px(icon.size.width, px(32.0));
    assert_px(icon.size.height, px(32.0));

    let confirm = find(&records, "search-replace-confirm");
    assert!(confirm.content_sized, "{confirm:?}");
    let all = find(&records, "search-replace-all");
    assert!(all.content_sized, "{all:?}");

    let confirm_text = confirm.text.expect("\"Replace\" paints a run");
    assert_eq!(confirm_text.content, "Replace");
    let all_text = all.text.expect("\"All\" paints a run");
    assert_eq!(all_text.content, "All");

    // Each button's box is its own run's width **plus this button family's
    // own padding and border** — not a bare `ceil(text_width)` the way
    // `search-toggle-icons`' unpadded `preserve-case` span is. See
    // `search::REPLACE_ACTION_PADDING_X`'s doc comment for the arithmetic
    // that overturned this test's own first draft.
    let chrome = f32::from(search::REPLACE_ACTION_PADDING_X) * 2.0
        + f32::from(search::REPLACE_ACTION_BORDER_WIDTH) * 2.0;
    assert_px(
        at_replace_row(&records, "search-replace-confirm")
            .size
            .width,
        px((f32::from(confirm_text.width) + chrome).ceil()),
    );
    assert_px(
        at_replace_row(&records, "search-replace-all").size.width,
        px((f32::from(all_text.width) + chrome).ceil()),
    );
}

/// **This overturns `button.rs`'s own "no live call site" claim** — see
/// `crowbar_ui::components::search`'s module docs. Pinned here as a
/// regression: a future edit that made these buttons author a fixed width
/// again would silently undo the finding.
#[gpui::test]
fn the_two_replace_buttons_are_genuinely_content_sized_not_fixed(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, replace_cell(&[]));

    let confirm = at_replace_row(&records, "search-replace-confirm");
    let all = at_replace_row(&records, "search-replace-all");
    assert_ne!(
        confirm.size.width, all.size.width,
        "\"Replace\" and \"All\" are different strings and must not share one fixed width",
    );
}

/// The light table paints a different shell — `--background` and `--border`
/// both move.
#[gpui::test]
fn the_light_table_paints_a_different_shell(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let root = find(&records, "search-popover");

    assert_eq!(
        root.background,
        Paint::Solid(
            Theme::LIGHT
                .background
                .mix(95.0, crowbar_ui::Color::TRANSPARENT)
                .value()
        ),
    );
    assert_ne!(Theme::LIGHT.background, Theme::DARK.background);
}
