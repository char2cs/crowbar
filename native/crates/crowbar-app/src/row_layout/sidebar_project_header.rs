//! `--surface sidebar-project-header`, laid out in a real window.
//!
//! What follows pins the arithmetic `crowbar_ui::surfaces::sidebar::
//! sidebar_project_header`'s module docs describe — the traffic-light
//! spacer's conditional presence, the mirror `--right` produces, the platform
//! height switch and the fact that a dimmed back/forward button still
//! occupies its own slot — and reproduces the live reference's own seven
//! numbers directly, through a real taffy layout rather than by trusting the
//! hand arithmetic in this surface's own module docs.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::surfaces::sidebar::sidebar_project_header;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "sidebar-project-header"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's own root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, sidebar_project_header::ID_SIDEBAR_PROJECT_HEADER, id)
}

/// **The default cell carries every contract anchor exactly once, and never
/// `sidebar-toggle-icon`** — the toggle's own glyph belongs to a different,
/// already-registered surface and this composition does not paint it (see
/// the component's module docs and `web/src/lib/oracle/extract.ts`'s own
/// `oracleSurfaceScope` entry for this surface).
#[gpui::test]
fn the_default_cell_carries_every_contract_anchor_and_no_toggle_icon(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        "sidebar-project-header",
        "sidebar-project-header-toggle",
        "sidebar-project-header-back",
        "sidebar-project-header-forward",
        "sidebar-project-header-settings",
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert!(
        !seen.iter().any(|id| id == "sidebar-toggle-icon"),
        "{seen:?} — the toggle's glyph must never be anchored on this surface",
    );
    assert_eq!(seen.len(), 5, "{seen:?}");
}

/// **The resting, left-docked macOS cell shows its traffic-light spacer**:
/// the toggle sits `PADDING_OUTER (12px)` off the left edge, past a 72px
/// spacer and one `GAP (4px)` — not flush against the edge, which is what a
/// missing spacer would produce.
///
/// **Mutation:** deleting `SidebarProjectHeader::shows_traffic_lights`'s own
/// `!self.is_right` half (leaving only `self.platform.is_mac()`) does not
/// redden this test, because the default cell already has `is_right: false`
/// — the case that catches it is
/// `the_right_docked_cell_reproduces_the_live_reference` below, where the
/// spacer would then wrongly appear and every one of that test's numbers
/// would fail.
#[gpui::test]
fn the_default_cell_shows_its_traffic_light_spacer(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320"]));

    let toggle = at(&records, "sidebar-project-header-toggle");
    // 12 (pl) + 72 (spacer) + 4 (gap) = 88.
    assert_px(toggle.origin.x, px(88.0));
    assert_px(toggle.origin.y, px(8.0));
    assert_px(toggle.size.width, px(28.0));
    assert_px(toggle.size.height, px(28.0));

    let back = at(&records, "sidebar-project-header-back");
    let forward = at(&records, "sidebar-project-header-forward");
    let settings = at(&records, "sidebar-project-header-settings");
    // 12 + 72 + 4 + 28 + 4 (gap) + 100 (flex-1 spacer) + 4 (gap) = 224.
    assert_px(back.origin.x, px(224.0));
    assert_px(forward.origin.x - back.origin.x, px(30.0)); // 28 + CLUSTER_GAP(2)
    assert_px(settings.origin.x - forward.origin.x, px(30.0));

    let root = find(&records, "sidebar-project-header");
    assert_px(root.bounds.size.height, px(44.0)); // HEIGHT_MAC
}

/// **`--platform other` disables the traffic lights even when left-docked**,
/// and shortens the bar to `HEIGHT_OTHER` (34px) — `shows_traffic_lights` is
/// a conjunction of *both* `is_mac()` and `!is_right`, checked here on the
/// axis `the_default_cell_shows_its_traffic_light_spacer` does not reach.
///
/// **Mutation:** replacing `self.platform.is_mac() && !self.is_right` with
/// `!self.is_right` alone in `SidebarProjectHeader::shows_traffic_lights`
/// turns this red — the toggle would then land at x = 88 (spacer present)
/// instead of x = 12 (spacer absent).
#[gpui::test]
fn platform_other_drops_the_spacer_and_shortens_the_bar(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "320", "--platform", "other"]));

    let toggle = at(&records, "sidebar-project-header-toggle");
    assert_px(toggle.origin.x, px(12.0)); // PADDING_OUTER, no spacer
    assert_px(toggle.origin.y, px(3.0)); // (34 - 28) / 2

    let root = find(&records, "sidebar-project-header");
    assert_px(root.bounds.size.height, px(34.0)); // HEIGHT_OTHER
    assert_ne!(root.bounds.size.height, px(44.0));
}

/// **The `--right` cell reproduces the live reference given for this item, to
/// the pixel**, through a real taffy layout rather than the hand arithmetic
/// in this surface's own module docs: `sidebar-project-header 344×44`,
/// `-toggle 304,8 28×28`, `-back 8,8 28×28`, `-forward 38,8 28×28`,
/// `-settings 68,8 28×28`.
///
/// **Mutation:** swapping `.pr(PADDING_OUTER).pl(PADDING_INNER)` for
/// `.pl(PADDING_OUTER).pr(PADDING_INNER)` in `SidebarProjectHeader::shell`'s
/// `is_right` arm turns every assertion below red — the cluster would start
/// at x = 12 instead of 8, and the toggle would land 4px short of 304.
#[gpui::test]
fn the_right_docked_cell_reproduces_the_live_reference(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "344", "--right"]));

    let root = find(&records, "sidebar-project-header");
    assert_px(root.bounds.size.width, px(344.0));
    assert_px(root.bounds.size.height, px(44.0));

    let toggle = at(&records, "sidebar-project-header-toggle");
    assert_px(toggle.origin.x, px(304.0));
    assert_px(toggle.origin.y, px(8.0));
    assert_px(toggle.size.width, px(28.0));
    assert_px(toggle.size.height, px(28.0));

    let back = at(&records, "sidebar-project-header-back");
    assert_px(back.origin.x, px(8.0));
    assert_px(back.origin.y, px(8.0));
    assert_px(back.size.width, px(28.0));
    assert_px(back.size.height, px(28.0));

    let forward = at(&records, "sidebar-project-header-forward");
    assert_px(forward.origin.x, px(38.0));
    assert_px(forward.origin.y, px(8.0));

    let settings = at(&records, "sidebar-project-header-settings");
    assert_px(settings.origin.x, px(68.0));
    assert_px(settings.origin.y, px(8.0));

    // And no traffic-light spacer at all — `is_right` disables it regardless
    // of platform, so the cluster's own leading edge lands flush at
    // `PADDING_INNER` (8px) rather than 8 + 72 + 4.
    assert_ne!(back.origin.x, px(84.0));
}

/// **A dimmed back/forward button is still `visible` and still occupies its
/// own slot** — `DISABLED_OPACITY` (0.64) is not zero, so
/// `native/oracle/ANCHORS.md` v1.7's opacity term does not fire, the
/// identical limitation `row_layout::button::
/// a_disabled_button_is_still_visible_to_both_extractors` already documents
/// for `button`'s own surface. Geometry is unaffected either way — this test
/// is the proof, not an assumption.
#[gpui::test]
fn disabled_back_and_forward_stay_visible_and_keep_their_slot(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let resting = measure(cx, cell(&["--width", "320"]));
    let dimmed = measure(cx, cell(&["--width", "320", "--no-back", "--no-forward"]));

    for id in [
        "sidebar-project-header-back",
        "sidebar-project-header-forward",
    ] {
        let before = find(&resting, id);
        let after = find(&dimmed, id);
        assert!(after.visible, "{id}");
        assert_eq!(before.bounds, after.bounds, "{id}");
    }
}

/// The bar's own width tracks `--width` exactly — `w(relative(1.0))` fills
/// whatever its parent gives it, not an authored length.
#[gpui::test]
fn the_bar_fills_whatever_width_it_is_given(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for width in [280u16, 320, 500] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        let root = find(&records, "sidebar-project-header");
        assert_px(root.bounds.size.width, px(f32::from(width)));
    }
}
