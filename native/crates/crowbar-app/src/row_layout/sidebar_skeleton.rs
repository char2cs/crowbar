//! `--surface sidebar-skeleton`, laid out in a real window.
//!
//! No reference exists to compare these against —
//! `crowbar_ui::surfaces::sidebar::sidebar_skeleton`'s own module docs record why
//! the composition can never mount. What follows instead pins the one thing
//! worth pinning: that eighteen bars across five row shapes actually stack
//! the way the source's three `[1, 2].map` calls say they should, through a
//! real taffy layout rather than the hand arithmetic in this surface's own
//! module docs.

use super::{a_cell, assert_px, find, ids, measure};
use crowbar_ui::surfaces::sidebar::sidebar_skeleton;
use gpui::{TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "sidebar-skeleton"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// **Every one of the eighteen bars plus the divider is recorded exactly
/// once, alongside the root** — twenty anchors, matching the brief's own
/// count ("root, 18 uniquely-indexed `Skeleton` ids, and a divider").
#[gpui::test]
fn every_anchor_appears_exactly_once_and_the_count_is_twenty(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    let mut sorted = seen.clone();
    sorted.sort_unstable();
    let before = sorted.len();
    sorted.dedup();
    assert_eq!(sorted.len(), before, "{seen:?}");
    assert_eq!(seen.len(), 20, "{seen:?}");

    for id in [
        "sidebar-skeleton",
        "sidebar-skeleton-chat-0-icon",
        "sidebar-skeleton-chat-0-title",
        "sidebar-skeleton-chat-0-meta",
        "sidebar-skeleton-chat-1-icon",
        "sidebar-skeleton-chat-1-title",
        "sidebar-skeleton-chat-1-meta",
        "sidebar-skeleton-divider",
        "sidebar-skeleton-repo-0-icon",
        "sidebar-skeleton-repo-0-name",
        "sidebar-skeleton-repo-0-workspace-0-title",
        "sidebar-skeleton-repo-0-workspace-0-meta",
        "sidebar-skeleton-repo-0-workspace-1-title",
        "sidebar-skeleton-repo-0-workspace-1-meta",
        "sidebar-skeleton-repo-1-icon",
        "sidebar-skeleton-repo-1-name",
        "sidebar-skeleton-repo-1-workspace-0-title",
        "sidebar-skeleton-repo-1-workspace-0-meta",
        "sidebar-skeleton-repo-1-workspace-1-title",
        "sidebar-skeleton-repo-1-workspace-1-meta",
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
}

/// **The whole column's own height is 321px at the default 320px width** —
/// `py-1` (4 + 4), five top-level children (two 36px chat rows, a 9px
/// divider counting its own `my-1` margin, two 112px repo groups) and four
/// `space-y-0.5` (2px) gaps between them, read off a real layout rather than
/// asserted from the arithmetic that predicts it.
///
/// **Mutation:** changing `OUTER_GAP` from `SPACING * 0.5` to `SPACING` in
/// `crowbar_ui::surfaces::sidebar::sidebar_skeleton` turns this red — four gaps
/// widen from 2px to 4px each, an 8px increase, so the root's own height
/// becomes 329px, not 321.
#[gpui::test]
fn the_columns_own_height_is_three_hundred_and_twenty_one_pixels(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320"]));
    let root = find(&records, sidebar_skeleton::ID_SIDEBAR_SKELETON);

    assert_px(root.bounds.size.height, px(321.0));
}

/// **The two chat rows are offset by exactly one row height plus the outer
/// gap** (36 + 2 = 38px) — the same spacing `space-y-0.5` between two `h-9`
/// siblings produces.
///
/// **Mutation:** hardcoding `ROW_HEIGHT` to `px(30.0)` while leaving every
/// other constant alone turns this red — the offset becomes 32px, not 38.
#[gpui::test]
fn the_two_chat_rows_are_offset_by_a_row_height_plus_the_outer_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320"]));

    let icon0 = find(&records, "sidebar-skeleton-chat-0-icon");
    let icon1 = find(&records, "sidebar-skeleton-chat-1-icon");
    assert_px(icon1.bounds.origin.y - icon0.bounds.origin.y, px(38.0));
    assert_px(icon0.bounds.origin.x, icon1.bounds.origin.x);
}

/// **Each repo group is a header row plus two workspace rows, each offset by
/// a row height plus the group's own `space-y-0.5` gap** (36 + 2 = 38px) —
/// and the workspace rows' own `pl-6` lands their title bar 24px off the
/// group's left edge, not the header row's 14px (`ROW_MARGIN_X` only, no
/// `pl-6`).
///
/// **Mutation:** deleting the `.pl(WORKSPACE_ROW_PADDING_LEFT)` branch in
/// `SidebarSkeleton::row_shell` turns the indentation assertion red — the
/// workspace title would land at the header row's own 14px inset instead of
/// 24.
#[gpui::test]
fn each_repo_group_is_a_header_and_two_indented_workspace_rows(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320"]));

    let header_icon = find(&records, "sidebar-skeleton-repo-0-icon");
    // The header's own **name** bar, not its icon: both it and the workspace
    // title bars below are `TEXT_BAR_HEIGHT` (12px), vertically centred in
    // their `h-9` (36px) row by the identical 12px offset — comparing two
    // bars of the *same* height cancels that centring term out of the row
    // spacing this test is actually about. Comparing against the icon (20px,
    // centred by 8px instead) would add a spurious 4px to every row-spacing
    // delta below.
    let header_name = find(&records, "sidebar-skeleton-repo-0-name");
    let ws0_title = find(&records, "sidebar-skeleton-repo-0-workspace-0-title");
    let ws1_title = find(&records, "sidebar-skeleton-repo-0-workspace-1-title");

    assert_px(
        ws0_title.bounds.origin.y - header_name.bounds.origin.y,
        px(38.0),
    );
    assert_px(ws1_title.bounds.origin.y - ws0_title.bounds.origin.y, px(38.0));

    // `pl-6` (24px) *replaces* the row's own `px-2` (8px) left half rather
    // than adding to it — a workspace row's own content starts 16px
    // (24 − 8) further right than the header row's does. Compared against
    // the header's own **icon** here, not its name bar: both the icon and
    // `ws0_title` are the *first* child of their own row, so both sit
    // exactly at their row's own content-start x, differing only by the `pl`
    // override — the name bar is the header row's *second* child and starts
    // 20px (icon) + 8px (gap) further right, which would not isolate the
    // same thing.
    assert_px(
        ws0_title.bounds.origin.x - header_icon.bounds.origin.x,
        px(16.0),
    );
}

/// **The two repo groups are offset by one group's own total height (112px)
/// plus the outer gap (2px)** — 114px, read between each group's own header
/// icon.
#[gpui::test]
fn the_two_repo_groups_are_offset_by_a_groups_own_height_plus_the_outer_gap(
    cx: &mut TestAppContext,
) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320"]));

    let repo0 = find(&records, "sidebar-skeleton-repo-0-icon");
    let repo1 = find(&records, "sidebar-skeleton-repo-1-icon");
    assert_px(repo1.bounds.origin.y - repo0.bounds.origin.y, px(114.0));
}

/// **The divider sits strictly between the two chat rows and the first repo
/// group**, is exactly `DIVIDER_HEIGHT` (1px) tall regardless of column
/// width, and paints no text of its own.
#[gpui::test]
fn the_divider_sits_between_the_chats_and_the_repos_and_is_one_pixel_tall(
    cx: &mut TestAppContext,
) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320"]));

    let chat1 = find(&records, "sidebar-skeleton-chat-1-icon");
    let divider = find(&records, "sidebar-skeleton-divider");
    let repo0 = find(&records, "sidebar-skeleton-repo-0-icon");

    assert_px(divider.bounds.size.height, px(1.0));
    assert!(divider.bounds.origin.y > chat1.bounds.origin.y);
    assert!(divider.bounds.origin.y < repo0.bounds.origin.y);
    assert!(divider.text.is_none());
    assert!(!divider.content_sized);
    assert!(!divider.line_sized);
}

/// **The two bars `skeleton::CallSite` does not carry stay fixed as the
/// column widens; the two `flex-1` title bars grow with it, 1:1.** This is
/// the row-layout half of the claim `crowbar_ui::surfaces::sidebar::
/// sidebar_skeleton`'s own module docs make about the two "not in
/// `CallSite`" shapes — a bar that hardcoded `w-24`/`w-12` in the browser and
/// was accidentally ported `flex-1` (or the reverse) would be caught here,
/// not by the unit-level constant check alone.
///
/// **Mutation:** replacing `Self::meta_bar(theme, REPO_NAME_WIDTH)` with
/// `Self::title_bar(theme)` in `SidebarSkeleton::repo_group` turns the repo
/// name assertion red — its width would grow with the column instead of
/// staying at 96px.
#[gpui::test]
fn fixed_bars_stay_fixed_and_flex_bars_grow_with_the_column(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let narrow = measure(cx, cell(&["--width", "320"]));
    let wide = measure(cx, cell(&["--width", "420"]));

    // Fixed: repo name (w-24, 96px), workspace meta (w-12, 48px), chat meta
    // (w-8, 32px) — identical at both widths.
    for id in [
        "sidebar-skeleton-repo-0-name",
        "sidebar-skeleton-repo-0-workspace-0-meta",
        "sidebar-skeleton-chat-0-meta",
    ] {
        let a = find(&narrow, id).bounds.size.width;
        let b = find(&wide, id).bounds.size.width;
        assert_px(a, b);
    }
    assert_px(find(&narrow, "sidebar-skeleton-repo-0-name").bounds.size.width, px(96.0));
    assert_px(
        find(&narrow, "sidebar-skeleton-repo-0-workspace-0-meta")
            .bounds
            .size
            .width,
        px(48.0),
    );
    assert_px(find(&narrow, "sidebar-skeleton-chat-0-meta").bounds.size.width, px(32.0));

    // Flex: chat title and workspace title grow by exactly the 100px column
    // delta.
    for id in [
        "sidebar-skeleton-chat-0-title",
        "sidebar-skeleton-repo-0-workspace-0-title",
    ] {
        let a = find(&narrow, id).bounds.size.width;
        let b = find(&wide, id).bounds.size.width;
        assert_px(b - a, px(100.0));
    }
}

/// Every icon bar is `h-5 w-5` (20 × 20) with `rounded-md`, regardless of
/// which row it is in.
#[gpui::test]
fn every_icon_bar_is_twenty_by_twenty_with_the_medium_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320"]));
    let theme = crowbar_ui::Theme::DARK;

    for id in [
        "sidebar-skeleton-chat-0-icon",
        "sidebar-skeleton-chat-1-icon",
        "sidebar-skeleton-repo-0-icon",
        "sidebar-skeleton-repo-1-icon",
    ] {
        let icon = find(&records, id);
        assert_px(icon.bounds.size.width, px(20.0));
        assert_px(icon.bounds.size.height, px(20.0));
        assert_px(icon.radius, theme.radius_md.value());
    }
}

