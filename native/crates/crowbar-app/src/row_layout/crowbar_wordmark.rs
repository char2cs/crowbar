//! `--surface crowbar-wordmark`: what taffy resolves the brand lockup to,
//! against the captured reference at `/tmp/p3-ref-crowbar-wordmark.json`.
//!
//! The reference's numbers are `148 × 37.56`, `bg #00000000`, `radius 0`,
//! `border.w 0`, `visible: true`.
//!
//! The height is the one derived quantity here — `width × 115 / 453` — and it is
//! where the two engines part. **Measured in this harness:** the exact ratio is
//! `37.5717`, `WebKit` floors it into `1/64`ths and reports `37.5625`, and taffy
//! snaps it to the device pixel grid and gives **`37.5`** at DPR 2. A `0.0625`
//! delta against a ±0.5 tolerance. The heights below are therefore asserted
//! **within tolerance of the exact ratio** rather than pinned, which is the same
//! spelling `a_percentage_length_lands_on_a_whole_pixel` uses for the same
//! reason.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::surfaces::crowbar_wordmark;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "crowbar-wordmark"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the lockup itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, crowbar_wordmark::ID_CROWBAR_WORDMARK, id)
}

/// **The default cell is the captured new-tab isologo.** The width is asserted
/// exactly — the clamp is arithmetic both engines do the same way — and the
/// height within `ANCHORS.md` §5's ±0.5, which is where the `1/64`px floor
/// lives.
#[gpui::test]
fn the_default_cell_is_the_captured_isologo(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["crowbar-wordmark".to_owned()]);

    let root = at(&records, "crowbar-wordmark");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(148.0));
    assert_within_tolerance(root.size.height, px(37.56));

    let record = find(&records, "crowbar-wordmark");
    assert!(record.visible);
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert_eq!(record.background, Paint::None);
    assert!(
        record.text.is_none(),
        "the lettering is path fill, not a run — the anchor carries no text group",
    );
    assert!(!record.content_sized);
    assert!(!record.line_sized);
}

/// The height **tracks the width through the viewBox's ratio**, which is the
/// one thing on this surface a port could get wrong independently of the clamp.
/// Three panes, three widths, and the ratio holds at each — within the ±0.5 the
/// module docs account for.
#[gpui::test]
fn the_height_follows_the_width_through_the_view_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    // 700 × 0.14 = 98, inside the clamp; 1073 × 0.14 overshoots the 148 ceiling;
    // 100 × 0.14 = 14, under the 96 floor.
    for (pane_min, expected_width) in [(700_u32, 98.0_f32), (1073, 148.0), (100, 96.0)] {
        let pane = pane_min.to_string();
        let records = measure(cx, cell(&["--pane-min", &pane]));
        let root = at(&records, "crowbar-wordmark");
        assert_px(root.size.width, px(expected_width));

        let ratio = crowbar_wordmark::VIEW_BOX_HEIGHT / crowbar_wordmark::VIEW_BOX_WIDTH;
        assert_within_tolerance(root.size.height, px(expected_width * ratio));
    }
}

/// **taffy snaps the derived height to the device pixel grid**, which is the
/// measurement behind the `0.0625` delta the module docs account for. Asserted
/// directly rather than left as prose, so that a gpui bump which stopped
/// snapping — or started snapping differently — fails here rather than
/// surfacing as a mystery delta on the one anchor this surface has.
///
/// The harness window runs at **DPR 2**, so the grid is `0.5`px: the exact
/// `37.5717` lands on `37.5`, against `WebKit`'s `37.5625`.
#[gpui::test]
fn taffy_snaps_the_derived_height_to_the_device_grid(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let height = f32::from(at(&records, "crowbar-wordmark").size.height);

    let doubled = height * 2.0;
    assert!(
        (doubled - doubled.round()).abs() < 1e-4,
        "a DPR-2 device pixel is 0.5px, so the height should be a multiple of it; got {height}",
    );

    // The exact ratio is NOT on that grid, so the assertion above is doing work
    // rather than coinciding with the arithmetic.
    let exact = 148.0 * crowbar_wordmark::VIEW_BOX_HEIGHT / crowbar_wordmark::VIEW_BOX_WIDTH;
    assert!(
        ((exact * 2.0) - (exact * 2.0).round()).abs() > 0.05,
        "{exact}"
    );

    // And the reference's own number — the same ratio floored into 1/64ths — is
    // a third value again, well inside ANCHORS.md §5's ±0.5.
    assert!(
        (height - 37.5625).abs() < 0.1,
        "{height} against the reference's 37.5625",
    );
}

/// The two OOBE lockups lay out, even though neither can be captured. Ported
/// and measured here so that "unreachable" means *no reference* rather than *no
/// implementation* — `git-row-dir`'s standing.
///
/// `--width 400` because the lockup is a flex item with the CSS default
/// `flex-shrink: 1`, exactly as the `<svg>` is, and the 320px surface default
/// would squeeze the 360px presentation lockup. That the port shrinks there is
/// itself faithful: the OOBE lockup's real container is the whole window.
#[gpui::test]
fn the_unreachable_oobe_lockups_still_lay_out(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (call_site, expected) in [("oobe-presentation", 360.0_f32), ("oobe-card", 180.0)] {
        let records = measure(
            cx,
            cell(&[
                "--call-site",
                call_site,
                "--pane-min",
                "1714",
                "--width",
                "400",
            ]),
        );
        let root = at(&records, "crowbar-wordmark");
        assert_px(root.size.width, px(expected));
        assert!(find(&records, "crowbar-wordmark").visible, "{call_site}");
    }
}

/// §8.3's `empty`: the width merged away. No area, and `visible` says so.
#[gpui::test]
fn the_empty_cell_has_no_area_and_reports_invisible(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));

    let root = at(&records, "crowbar-wordmark");
    assert_px(root.size.width, px(0.0));
    assert_px(root.size.height, px(0.0));
    assert!(!find(&records, "crowbar-wordmark").visible);
}

/// **Every recorded field is theme-invariant.** Same finding as the mark's, and
/// the same control: a port that gave the lockup a paint would part here.
#[gpui::test]
fn the_two_appearances_record_the_same_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark = find(&measure(cx, cell(&["--theme", "dark"])), "crowbar-wordmark");
    let light = find(
        &measure(cx, cell(&["--theme", "light"])),
        "crowbar-wordmark",
    );
    assert_px(dark.bounds.size.width, light.bounds.size.width);
    assert_px(dark.bounds.size.height, light.bounds.size.height);
    assert_eq!(dark.background, light.background);
    assert_eq!(dark.visible, light.visible);
}
