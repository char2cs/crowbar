//! **`tabs`, and the one claim the port cannot make without a window.**
//!
//! Every other surface here is measured because taffy and `WebKit` might divide
//! space differently. This one is measured because the port makes a *structural*
//! claim: base-ui positions `Tabs.Indicator` from a runtime measurement of the
//! active tab, written into six CSS custom properties, and gpui has neither
//! custom properties nor a way for a `Div` to read a sibling's laid-out box — so
//! `crowbar_ui::primitives::tabs` renders the indicator as an absolutely
//! positioned child of the active tab, inset by the tab's own border width.
//!
//! That is only correct if an inset resolves against the containing block's
//! **padding** box, as CSS says it does. Nothing in the source proves taffy
//! agrees. These assertions do, and they are what stops the alternative — taking
//! the indicator's box as a parameter — from being the easier answer: a
//! parameter would make `tab-indicator`'s bounds a copy of the reference's own
//! numbers, on the one anchor this item exists to get right.
//!
//! The numbers are the live sidebar tab bar's, read off the running app at
//! `innerWidth` 1714 with a 278px root: tabs at `2,2,90,32`, `94,2,90,32`,
//! `186,2,90,32` and the indicator at `2,2,90,32`.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::primitives::tabs;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
///
/// `--width 278` and `--viewport-width 1714` are the live reference's own
/// geometry: the sidebar's content box inside a window much wider than it.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "tabs",
        "--width",
        "278",
        "--viewport-width",
        "1714",
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, tabs::ID_ROOT, id)
}

/// The live tab bar's three ids, in DOM order.
const WORKSPACES: &str = "tabs-tab-workspaces";
const CHATS: &str = "tabs-tab-chats";
const FILES: &str = "tabs-tab-files";

/// Anchor presence is what the differ ranks first.
#[gpui::test]
fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        tabs::ID_ROOT,
        tabs::ID_LIST,
        WORKSPACES,
        CHATS,
        FILES,
        tabs::ID_INDICATOR,
    ] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    assert_eq!(seen.len(), 6, "{seen:?}");

    // No panel: `sidebar-tab-bar.tsx` renders none, and an anchor the
    // reference cannot produce is a `FieldPresence` delta that forgives
    // nothing. Nor a bare `tabs-tab` — every tab's id is its own value.
    assert!(!seen.iter().any(|id| id.starts_with("tabs-content")));
    assert!(!seen.iter().any(|id| id == "tabs-tab"));
    // And nothing from another surface leaked in.
    assert!(
        !seen.iter().any(|id| id.starts_with("git-row-")
            || id.starts_with("file-row-")
            || id.starts_with("menu-")
            || id.starts_with("resize-")
            || id.starts_with("carousel-")),
        "{seen:?}",
    );
}

/// **The indicator's box is the active tab's box.** The whole port of this
/// component rests on it, and it is a measurement rather than a claim.
///
/// Exact, not `± 0.5`: both boxes come out of the *same* taffy layout, so a
/// tolerance here would only hide the inset being wrong.
#[gpui::test]
fn the_indicator_is_exactly_the_active_tabs_border_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (line, active) in [
        (vec![], WORKSPACES),
        (vec!["--flags", "selected"], FILES),
        (vec!["--flags", "selected", "--active", "chats"], CHATS),
    ] {
        let records = measure(cx, cell(&line));
        let tab = at(&records, active);
        let indicator = at(&records, tabs::ID_INDICATOR);

        assert_px(indicator.origin.x, tab.origin.x);
        assert_px(indicator.origin.y, tab.origin.y);
        assert_px(indicator.size.width, tab.size.width);
        assert_px(indicator.size.height, tab.size.height);

        // The inset is doing work rather than being zero by luck. The tab
        // really carries a 1px border, so `inset: 0` would have landed the
        // indicator on the tab's *padding* box — two pixels narrower and
        // shorter than what is asserted above — and that is exactly the
        // pixel `border.w` is compared for.
        assert_px(find(&records, active).border_width, tabs::TAB_BORDER);
        assert_px(tabs::INDICATOR_INSET, tabs::TAB_BORDER * -1.0);
        assert!(tabs::INDICATOR_INSET < px(0.0));
    }
}

/// **The tabs tile the list exactly**, at the live reference's numbers.
///
/// The list's `p-0.5` inset, then three equal `flex-1` shares separated by
/// the 2px `gap-x-0.5`, ending flush with the list's padding box.
#[gpui::test]
fn the_tabs_tile_the_list_at_the_references_own_numbers(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let root = at(&records, tabs::ID_ROOT);
    let list = at(&records, tabs::ID_LIST);

    // The root is the call site's `w-full` and the list is `w-full` of that,
    // so both are the surface width; the height is one tab plus the padding.
    assert_px(root.size.width, px(278.0));
    assert_px(list.size.width, px(278.0));
    assert_px(list.origin.x, px(0.0));
    assert_px(list.origin.y, px(0.0));
    assert_px(
        list.size.height,
        tabs::TAB_HEIGHT_SM + tabs::LIST_PADDING * 2.0,
    );
    assert_px(list.size.height, px(36.0));

    let boxes = [
        at(&records, WORKSPACES),
        at(&records, CHATS),
        at(&records, FILES),
    ];
    for tab in boxes {
        assert_px(tab.origin.y, tabs::LIST_PADDING);
        assert_px(tab.size.height, tabs::TAB_HEIGHT_SM);
        assert_px(tab.size.width, px(90.0));
    }
    assert_px(boxes[0].origin.x, tabs::LIST_PADDING);
    assert_px(boxes[1].origin.x, px(94.0));
    assert_px(boxes[2].origin.x, px(186.0));

    // No gap and no overlap, and the last tab ends flush with the padding
    // box — the property a port that rounded each share independently would
    // get wrong.
    for pair in boxes.windows(2) {
        assert_px(
            pair[1].origin.x - (pair[0].origin.x + pair[0].size.width),
            tabs::LIST_GAP,
        );
    }
    assert_px(
        boxes[2].origin.x + boxes[2].size.width + tabs::LIST_PADDING,
        list.size.width,
    );
}

/// **No active tab, no indicator.** base-ui renders none when the root has
/// no value, and an anchor present on one side only is a delta whose cause
/// would be this port rather than the UI.
#[gpui::test]
fn an_inactive_tab_bar_has_no_indicator_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(
        cx,
        cell(&["--flags", "selected", "--active", "missing"]),
    ));

    assert!(!seen.contains(&tabs::ID_INDICATOR.to_owned()), "{seen:?}");
    assert_eq!(seen.len(), 5, "{seen:?}");
    // The tabs are all still there, which is what makes this a missing
    // indicator rather than a missing list.
    for id in [WORKSPACES, CHATS, FILES] {
        assert!(seen.contains(&id.to_owned()), "{seen:?}");
    }
}

/// **The `underline` variant puts the rule below the list**, not under the
/// tab — because `data-[orientation=horizontal]:translate-y-px` replaces the
/// measured `-translate-y-(--active-tab-bottom)` on the same property rather
/// than adding to it. The arm has no live call site, so this is the only
/// place the arithmetic is checked at all.
#[gpui::test]
fn the_underline_sits_one_pixel_below_the_list(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--variant", "underline"]));

    let list = at(&records, tabs::ID_LIST);
    let tab = at(&records, WORKSPACES);
    let rule = at(&records, tabs::ID_INDICATOR);

    // Its span is the active tab's, on the axis the rule runs along.
    assert_px(rule.origin.x, tab.origin.x);
    assert_px(rule.size.width, tab.size.width);
    // Its thickness is authored, and its bottom edge is one pixel past the
    // list's.
    assert_px(rule.size.height, tabs::UNDERLINE_THICKNESS);
    assert_px(
        rule.origin.y + rule.size.height,
        list.origin.y + list.size.height + tabs::UNDERLINE_NUDGE,
    );
    // Which is *below* the tab, unlike the default variant's.
    assert!(rule.origin.y > tab.origin.y + tab.size.height);

    // And the list paints no background at all under this variant: the
    // `underline` arm of the `cn(...)` carries no `bg-*` utility, so the
    // call site's `--list-bg` reaches nothing.
    assert_eq!(
        find(&records, tabs::ID_LIST).background,
        crowbar_driver::Paint::None,
    );
    // Which the `default` arm does not — or the variant would be paint-free
    // on both sides and the assertion above would be about nothing.
    assert_ne!(
        find(&measure(cx, cell(&[])), tabs::ID_LIST).background,
        crowbar_driver::Paint::None,
    );
}
