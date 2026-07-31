//! The Phase 2 clipping surface: `--surface sidebar-carousel`.
//!
//! A module rather than a second file, for the reason the two above are: it
//! shares `measure`, and a second copy of the harness is a second thing to
//! drift. What it does not share is the thing it measures — both Phase 1
//! surfaces and the menu fit inside their own boxes, and **three quarters of
//! this one is outside its own box at every instant**. So every assertion here
//! is about a track wider than the viewport it is seen through, and about the
//! one field that says so.

use super::{a_cell, assert_px, find, ids, measure};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::sidebar_carousel::{ID_SCROLLPORT, SidebarTab, TABS};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// The sidebar width the live measurement was taken at: `clientWidth` 294
/// in a 600px window, with a `scrollWidth` of 1176.
const SURFACE_WIDTH: f32 = 294.0;

/// `--height`'s default, and therefore every panel's height.
const HEIGHT: f32 = 600.0;

/// A cell on this surface, at the width the reference was measured at.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "sidebar-carousel",
        "--width",
        "294",
        "--viewport-width",
        "600",
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root — the scrollport.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    super::relative_to(records, ID_SCROLLPORT, id)
}

/// Every anchor this surface has, root first and then the track in DOM
/// order.
fn every_anchor() -> Vec<&'static str> {
    let mut all = vec![ID_SCROLLPORT];
    all.extend(TABS.into_iter().map(SidebarTab::anchor));
    all
}

/// Anchor presence is what the differ ranks first, so a missing one is the
/// loudest possible failure — and an *extra* one is an anchor the reference
/// cannot produce, which forgives nothing either.
#[gpui::test]
fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    assert_eq!(ids(&measure(cx, cell(&[]))), every_anchor());
}

/// **The live measurement, reproduced.** At the sidebar's 294px the track is
/// four scrollports — 1176px — wide, one of which is in view. That pair of
/// numbers came off the running app, so this is the one assertion here that
/// is checked against something other than gpui.
#[gpui::test]
fn the_track_is_four_scrollports_wide_and_shows_one(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let scrollport = at(&records, ID_SCROLLPORT);
    assert_px(scrollport.size.width, px(SURFACE_WIDTH));
    assert_px(scrollport.origin.x, px(0.0));

    for tab in TABS {
        let panel = at(&records, tab.anchor());
        // `min-w-full`, and it is a floor that every panel is pinned *at*:
        // four of them against one scrollport of space is negative free
        // space, so flexbox freezes each at its minimum.
        assert_px(panel.size.width, px(SURFACE_WIDTH));
        assert_px(panel.origin.x, px(f32::from(tab.index()) * SURFACE_WIDTH));
    }

    // The track's right edge: 4 × 294 = 1176 against a 294px scrollport.
    let last = at(&records, SidebarTab::Git.anchor());
    assert_px(last.origin.x + last.size.width, px(4.0 * SURFACE_WIDTH));
    assert_px(px(4.0 * SURFACE_WIDTH), px(1176.0));
}

/// **The field this surface exists for.** A panel scrolled out of the
/// scrollport reports `visible: false` and keeps every pixel of its
/// geometry, so a wrong scroll offset cannot hide behind the flag.
///
/// The tangent case is asserted by name: at `scrollLeft = 2W` the fourth
/// panel begins **exactly** at the scrollport's right edge, the intersection
/// is zero-wide, and that has to read as invisible — the DOM side requires
/// `r - l > 0` and the driver requires a non-empty intersection, which is
/// the same answer arrived at twice.
#[gpui::test]
fn a_snapped_out_panel_is_invisible_and_keeps_its_geometry(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "selected"]));

    // `--active-tab` defaults to `files`, index 2.
    let expected = [
        (SidebarTab::Workspaces, -2.0, false),
        (SidebarTab::Chats, -1.0, false),
        (SidebarTab::Files, 0.0, true),
        (SidebarTab::Git, 1.0, false),
    ];
    for (tab, scrollports, visible) in expected {
        let panel = at(&records, tab.anchor());
        assert_px(panel.origin.x, px(scrollports * SURFACE_WIDTH));
        assert_px(panel.size.width, px(SURFACE_WIDTH));
        assert_eq!(
            find(&records, tab.anchor()).visible,
            visible,
            "{}",
            tab.name(),
        );
    }

    // The tangency, spelled out: the fourth panel's left edge is the
    // scrollport's right edge, so the overlap is exactly zero.
    let scrollport = at(&records, ID_SCROLLPORT);
    let git = at(&records, SidebarTab::Git.anchor());
    assert_px(git.origin.x, scrollport.origin.x + scrollport.size.width);
    assert!(!find(&records, SidebarTab::Git.anchor()).visible);

    // And the root is present, at the origin, and visible —
    // `ANCHORS.md` v1.1 §4 makes all three a load error otherwise.
    assert_px(scrollport.origin.x, px(0.0));
    assert_px(scrollport.origin.y, px(0.0));
    assert!(find(&records, ID_SCROLLPORT).visible);
}

/// Every tab snaps the track by **exactly one scrollport per index**, and
/// leaves exactly one panel visible. This is `scrollLeft = index *
/// clientWidth` measured rather than restated.
#[gpui::test]
fn every_tab_snaps_the_track_by_one_scrollport_per_index(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for active in TABS {
        let records = measure(
            cx,
            cell(&["--flags", "selected", "--active-tab", active.name()]),
        );

        for tab in TABS {
            let offset = f32::from(tab.index()) - f32::from(active.index());
            assert_px(
                at(&records, tab.anchor()).origin.x,
                px(offset * SURFACE_WIDTH),
            );
            assert_eq!(
                find(&records, tab.anchor()).visible,
                tab == active,
                "{} while showing {}",
                tab.name(),
                active.name(),
            );
        }

        let visible = TABS
            .into_iter()
            .filter(|tab| find(&records, tab.anchor()).visible)
            .count();
        assert_eq!(visible, 1, "showing {}", active.name());
    }
}

/// **The claim that licenses drawing the four panels empty, measured.**
///
/// The port renders the panels with nothing in them because
/// `WorkspaceTree`, `AgentChatsPanel`, `FileExplorerTree` and `GitPanel`
/// have no native equivalent. That is only sound if a panel's box does not
/// depend on what is inside it — and it does not, because all four are
/// `min-w-full`, so the track always overflows and every item is frozen at
/// its minimum whatever its flex base size was.
///
/// Driving a filler several times the surface width through every panel and
/// getting a byte-identical record set is that argument turned into a
/// measurement.
#[gpui::test]
fn what_is_inside_a_panel_does_not_move_the_track(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let empty = measure(cx, cell(&["--flags", "selected"]));

    for filler in ["1", "1200", "4000"] {
        let stuffed = measure(
            cx,
            cell(&["--flags", "selected", "--panel-content", filler]),
        );

        // The whole record, not just its geometry: `bg`, `radius`,
        // `border` and `visible` all have to be untouched too, or "the
        // track ignores its contents" would be a claim about four fields
        // out of eight.
        assert_eq!(stuffed, empty, "{filler}");
        for id in ids(&empty) {
            assert_eq!(at(&stuffed, &id), at(&empty, &id), "{id} at {filler}");
            assert_eq!(
                find(&stuffed, &id).visible,
                find(&empty, &id).visible,
                "{id} at {filler}",
            );
        }
    }
}

/// `h-full` is on the first two panels and not on the last two, and that
/// asymmetry is **inert**: a flex item in a row stretches to the container's
/// height, and `height: 100%` resolves against the same box. Measured here
/// rather than argued, because it is the only place in this port where the
/// JSX carries an inconsistency the port reproduces on purpose.
#[gpui::test]
fn h_full_and_the_default_stretch_give_the_same_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "selected"]));

    assert_px(at(&records, ID_SCROLLPORT).size.height, px(HEIGHT));
    for tab in TABS {
        let panel = at(&records, tab.anchor());
        assert_px(panel.size.height, px(HEIGHT));
        assert_px(panel.origin.y, px(0.0));
    }
    // The two that carry `h-full` and the two that do not, side by side.
    assert_eq!(
        TABS.map(SidebarTab::full_height),
        [true, true, false, false],
    );
}

/// `--height` is `NavStack`'s column, and it reaches every panel — which is
/// what makes a parity run able to match the reference's sidebar rather than
/// report an `h` delta on all five anchors.
#[gpui::test]
fn the_height_option_is_the_column_and_reaches_every_panel(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (option, expected) in [("120", 120.0), ("640", 640.0)] {
        let records = measure(cx, cell(&["--height", option]));
        for id in every_anchor() {
            assert_px(at(&records, id).size.height, px(expected));
        }
    }
}

/// The surface width scales the whole track: the panels are the scrollport,
/// and the track is four of it. The §8.3 width axis is the one axis that
/// does real work here.
#[gpui::test]
fn the_width_axis_scales_the_scrollport_and_the_track(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (width, expected) in [("240", 240.0), ("294", 294.0), ("420", 420.0)] {
        let records = measure(
            cx,
            a_cell(&[
                "--surface",
                "sidebar-carousel",
                "--width",
                width,
                "--viewport-width",
                "800",
                "--flags",
                "selected",
            ]),
        );

        assert_px(at(&records, ID_SCROLLPORT).size.width, px(expected));
        for tab in TABS {
            let panel = at(&records, tab.anchor());
            assert_px(panel.size.width, px(expected));
            // Two scrollports left of the origin at `--active-tab files`,
            // so the offset follows the width rather than being pinned.
            assert_px(
                panel.origin.x,
                px((f32::from(tab.index()) - 2.0) * expected),
            );
        }
    }
}

/// **This component paints nothing**, and that is a fact the differ can
/// check: `bg` is transparent on all five anchors, `radius` is zero, and
/// `border.w` — the one field `ANCHORS.md` v1.1 compares *exactly* — is
/// zero. A port that reached for a background here would be told so on every
/// cell of the matrix.
#[gpui::test]
fn nothing_on_this_surface_paints(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "selected"]));

    for id in every_anchor() {
        let record = find(&records, id);
        assert_eq!(record.background, Paint::None, "{id}");
        assert_px(record.radius, px(0.0));
        assert_px(record.border_width, px(0.0));
        assert_eq!(record.border_color, None, "{id}");
        // And no text: `text_width`, `clipped` and `font` are absent, which
        // is why both declaration lists are empty.
        assert!(record.text.is_none(), "{id}");
    }
}

/// **Two of §8.3's four axes cannot move an anchor on this surface**, and
/// saying so is worth a test rather than a sentence: the component carries
/// no colour, so `--theme` selects a token table nothing reads, and no text,
/// so the three `--content` lengths are one picture. A future change that
/// gave the carousel a background would fail here, which is the point.
#[gpui::test]
fn the_theme_and_content_axes_move_nothing(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let baseline = measure(cx, cell(&["--flags", "selected"]));

    for line in [
        vec!["--theme", "light"],
        vec!["--theme", "dark"],
        vec!["--content", "short"],
        vec!["--content", "overflow"],
    ] {
        let mut full = vec!["--flags", "selected"];
        full.extend_from_slice(&line);
        let records = measure(cx, cell(&full));

        assert_eq!(ids(&records), ids(&baseline), "{line:?}");
        for id in every_anchor() {
            assert_eq!(at(&records, id), at(&baseline, id), "{id} at {line:?}");
            assert_eq!(
                find(&records, id).background,
                find(&baseline, id).background,
                "{id} at {line:?}",
            );
            assert_eq!(
                find(&records, id).visible,
                find(&baseline, id).visible,
                "{id} at {line:?}",
            );
        }
    }
}
