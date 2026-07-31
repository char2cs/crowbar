//! The second Phase 2 surface: `--surface resizable`.
//!
//! A module rather than a second file, for the reason the two above are: it shares
//! `measure`, and a second copy of the harness is a second thing to drift. What it
//! does not share is what it measures. Both Phase 1 surfaces and the menu are
//! *stacks of authored lengths*; this one is a **division**. `flex-basis: 0` with
//! fractional `flex-grow` around a fixed 1px sibling means every panel's width is
//! one arithmetic operation on the group's, and the only honest way to know
//! whether taffy performs it the way `WebKit` does is to lay it out and read the
//! bounds back.
//!
//! It is also the surface with nothing else to measure — no text, and no colour
//! unless `--with-handle` is passed — which is asserted here rather than left as a
//! claim in the module docs.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::resizable;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "resizable"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, resizable::ID_GROUP, id)
}

/// The three ids the live group carries, in DOM order.
const SIDEBAR: &str = "resize-panel-sidebar";
const HANDLE: &str = "resize-handle";
const CONTENT: &str = "resize-panel-content";

/// Anchor presence is what the differ ranks first, so a missing one is the
/// loudest possible failure.
#[gpui::test]
fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in ["resize-group", SIDEBAR, HANDLE, CONTENT] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    // The live shell passes no `withHandle`, so the grip must not appear: an
    // anchor the reference cannot produce is a `FieldPresence` delta that
    // forgives nothing. Nor may the primitive's own default panel id, which
    // the two call-site names replace.
    for id in ["resize-handle-grip", "resize-panel"] {
        assert!(!seen.contains(&id.to_owned()), "{id} should be absent");
    }
    // And nothing from another surface leaked in. Two roots in one frame
    // would make `Snapshot::build` anchor to whichever it found first.
    assert!(
        !seen.iter().any(|id| id.starts_with("git-row-")
            || id.starts_with("file-row-")
            || id.starts_with("menu-")),
        "{seen:?}",
    );
}

/// **The group is the surface, and the shell box above it is not in the
/// snapshot.** `width: 100%` resolves against the cell's surface width and
/// `height: 100%` against `--shell-height`, which is the whole reason that
/// parameter exists — a percentage against an auto-height parent computes to
/// `auto`, and every bound here would be zero.
#[gpui::test]
fn the_group_fills_the_surface_and_the_shell_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let subject = cell(&["--width", "600", "--shell-height", "120"]);
    let records = measure(cx, subject.clone());
    let group = at(&records, "resize-group");

    assert_px(group.origin.x, px(0.0));
    assert_px(group.origin.y, px(0.0));
    assert_px(group.size.width, subject.width_px());
    assert_px(group.size.height, px(120.0));

    // The shell box carries no anchor, so it cannot reach a snapshot at all.
    assert!(!ids(&records).iter().any(|id| id.contains("shell")));
}

/// **The division, measured.** Three boxes tile the group's main axis exactly:
/// the sidebar from the origin, the separator's single pixel, then the content
/// panel to the far edge. No gap and no overlap, at a width where the share is
/// *not* a whole number — which is the case a port that rounded each panel
/// independently would get wrong.
#[gpui::test]
fn the_panels_and_the_separator_tile_the_group_exactly(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in ["320", "600", "1100"] {
        let records = measure(cx, cell(&["--width", width, "--viewport-width", "1200"]));
        let group = at(&records, "resize-group");
        let sidebar = at(&records, SIDEBAR);
        let handle = at(&records, HANDLE);
        let content = at(&records, CONTENT);

        assert_px(sidebar.origin.x, px(0.0));
        assert_px(handle.origin.x, sidebar.size.width);
        assert_px(handle.size.width, resizable::HANDLE_THICKNESS);
        assert_px(
            content.origin.x,
            handle.origin.x + resizable::HANDLE_THICKNESS,
        );
        assert_px(content.origin.x + content.size.width, group.size.width);

        // The separator takes its pixel out of the free space before the
        // division, because `flex: 0 0 auto` keeps it out of the growth.
        assert_px(
            sidebar.size.width + content.size.width + resizable::HANDLE_THICKNESS,
            group.size.width,
        );

        // Every child stretches to the group's cross axis: the group sets no
        // `align-items`, so CSS's `stretch` is in force on both sides.
        for id in [SIDEBAR, HANDLE, CONTENT] {
            let child = at(&records, id);
            assert_px(child.origin.y, px(0.0));
            assert_px(child.size.height, group.size.height);
        }
    }
}

/// **The share, against the CSS arithmetic.** `flex-basis: 0` makes each
/// panel's used width `grow / Σgrow × free`, where `free` is the group less the
/// separator's pixel. Spelled as the division rather than as a literal, so a
/// change to the fixture's factors cannot pass by leaving a number alone.
///
/// Exact where the arithmetic lands on a whole pixel, and inside
/// `native/oracle/ANCHORS.md` §5's ±0.5 where it does not — which is the
/// honest statement, because the two engines are entitled to quantise it
/// differently and the contract says so.
#[gpui::test]
fn each_panels_width_is_its_share_of_the_free_space(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    // An even split of a whole number of pixels: 321 − 1 = 320, half each.
    let even = measure(cx, cell(&["--width", "321", "--grow", "50,50"]));
    assert_px(at(&even, SIDEBAR).size.width, px(160.0));
    assert_px(at(&even, CONTENT).size.width, px(160.0));

    // The live pair, at a width where the share is fractional.
    let width = 600.0_f32;
    let free = width - f32::from(resizable::HANDLE_THICKNESS);
    let total = resizable::SIDEBAR_GROW + resizable::CONTENT_GROW;
    let records = measure(cx, cell(&["--width", "600", "--viewport-width", "700"]));

    assert_within_tolerance(
        at(&records, SIDEBAR).size.width,
        px(free * resizable::SIDEBAR_GROW / total),
    );
    assert_within_tolerance(
        at(&records, CONTENT).size.width,
        px(free * resizable::CONTENT_GROW / total),
    );

    // And a zero factor collapses its panel outright, because the basis is
    // zero and there is nothing else to size it from. **This is a state the
    // reference really has**: the shell's sidebar is `collapsible` with
    // `collapsedSize={0}`, so a collapsed sidebar is `--grow 0,100`.
    let collapsed = measure(cx, cell(&["--width", "600", "--grow", "0,100"]));
    assert_px(at(&collapsed, SIDEBAR).size.width, px(0.0));
    assert_px(at(&collapsed, CONTENT).size.width, px(free));
    // Both extractors call a zero-area box invisible — `oracleIsVisible`
    // requires `width > 0 && height > 0` and `crowbar-driver`'s `is_visible`
    // requires a non-empty intersection with the clip — so this cell compares
    // rather than diverging on a field neither side agreed the meaning of.
    assert!(!find(&collapsed, SIDEBAR).visible);
    assert!(find(&collapsed, CONTENT).visible);
}

/// **taffy rounds a panel's width to a whole logical pixel; `WebKit` does
/// not.** Recorded as a measurement rather than as prose, because it is the
/// systematic difference every parity run on this surface will meet.
///
/// At 600px with the live factors the CSS arithmetic is **146.8748**, and taffy
/// lays the panel out at exactly **147** — a *round*, so the error is bounded
/// by 0.5 in both directions and `native/oracle/ANCHORS.md` §5's ±0.5 already
/// covers it. It is not the `content_sized` case: that is a one-directional
/// `ceil` on a text run, which is why v1.5 had to correct for it rather than
/// tolerate it.
///
/// **The caution it does buy**, said here so a later reader does not have to
/// re-derive it: a share that lands on a half pixel is Δ 0.5 exactly, right at
/// the tolerance edge, and would read as a defect. Picking `--width` and
/// `--grow` so the arithmetic lands near a whole pixel is how a matrix cell
/// avoids reporting quantisation.
#[gpui::test]
fn taffy_rounds_a_panels_width_where_webkit_would_keep_the_fraction(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--width", "600", "--viewport-width", "700"]));

    let free = 600.0 - f32::from(resizable::HANDLE_THICKNESS);
    let total = resizable::SIDEBAR_GROW + resizable::CONTENT_GROW;
    let exact = free * resizable::SIDEBAR_GROW / total;

    // The CSS value is fractional…
    assert!((exact - 146.8748).abs() < 0.001, "{exact}");
    assert!(exact.fract() > 0.001, "{exact}");
    // …and taffy's is not.
    let measured = at(&records, SIDEBAR).size.width;
    assert_px(measured, px(147.0));
    assert!(f32::from(measured).fract() < f32::EPSILON, "{measured:?}");
    // Inside the contract's tolerance, and a round rather than a ceil: the
    // native side came out *above* here and must be free to come out below.
    assert!((f32::from(measured) - exact).abs() <= 0.5, "{measured:?}");
}

/// **The separator does not grow and does not shrink**, which is what keeps it
/// one pixel at every width — and is the property `flex: 0 0 auto` buys.
#[gpui::test]
fn the_separator_stays_one_pixel_at_every_width(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in ["320", "600", "1100"] {
        let records = measure(cx, cell(&["--width", width, "--viewport-width", "1200"]));
        let handle = find(&records, HANDLE);

        assert_px(handle.bounds.size.width, px(1.0));
        // It paints nothing: the visible divider a user sees is the `::after`
        // hit strip on hover, and at rest the separator is a transparent
        // 1px column. `border.w` is the field `ANCHORS.md` v1.1 compares
        // exactly, and `focus-visible:ring-1` is a shadow rather than a
        // border, so it is zero in every cell.
        assert_eq!(handle.background, Paint::None);
        assert_px(handle.border_width, px(0.0));
        assert_px(handle.radius, px(0.0));
    }
}

/// **A vertical group swaps the axes**, including the separator's: its
/// `aria-orientation` is the *opposite* of the group's, which is why the
/// `aria-[orientation=horizontal]:h-px w-full` rules are the ones a vertical
/// group gets.
///
/// No live call site asks for it, so this measures the port against its own
/// arithmetic rather than against a reference — said here so nobody reads it
/// as a parity result.
#[gpui::test]
fn a_vertical_group_divides_the_height_instead(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(
        cx,
        cell(&[
            "--orientation",
            "vertical",
            "--width",
            "600",
            "--shell-height",
            "121",
            "--grow",
            "50,50",
        ]),
    );

    let group = at(&records, "resize-group");
    let sidebar = at(&records, SIDEBAR);
    let handle = at(&records, HANDLE);
    let content = at(&records, CONTENT);

    // 121 − 1 = 120, half each.
    assert_px(group.size.height, px(121.0));
    assert_px(sidebar.size.height, px(60.0));
    assert_px(content.size.height, px(60.0));
    assert_px(handle.size.height, px(1.0));

    // The separator is now a full-width rule, and the panels stack.
    assert_px(handle.size.width, group.size.width);
    assert_px(sidebar.origin.y, px(0.0));
    assert_px(handle.origin.y, sidebar.size.height);
    assert_px(content.origin.y, handle.origin.y + px(1.0));
    for id in [SIDEBAR, HANDLE, CONTENT] {
        assert_px(at(&records, id).origin.x, px(0.0));
        assert_px(at(&records, id).size.width, group.size.width);
    }
}

/// **The grip is the only paint on this surface**, and it is the only reason
/// the theme axis is not entirely vacuous. `rounded-lg` is 10px here because
/// this project redefines `--radius-lg`, on a box 4px wide.
///
/// It has **no reference**: no live call site passes `withHandle`, so this
/// measures the port against its own arithmetic. Kept for the same reason
/// Phase 1 kept `git-row-dir` — the day a call site passes the prop, the cell
/// is already drawable.
#[gpui::test]
fn the_grip_is_a_four_by_twenty_four_pill_in_the_border_colour(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--with-handle", "--shell-height", "120"]));
    let grip = find(&records, "resize-handle-grip");
    let handle = at(&records, HANDLE);
    let grip_bounds = at(&records, "resize-handle-grip");

    assert_px(grip.bounds.size.width, resizable::GRIP_THICKNESS);
    assert_px(grip.bounds.size.height, resizable::GRIP_LENGTH);
    assert_px(grip.bounds.size.width, px(4.0));
    assert_px(grip.bounds.size.height, px(24.0));
    assert_px(grip.radius, Theme::DARK.radius_lg.value());
    assert_px(grip.radius, px(10.0));
    assert_eq!(grip.background, Paint::Solid(Theme::DARK.border.value()));

    // `items-center justify-center` on a 1px-wide separator: the grip
    // overflows it symmetrically, 1.5px either side, and is centred on the
    // group's cross axis.
    assert_px(grip_bounds.origin.x, handle.origin.x + px(0.5) - px(2.0));
    assert_px(
        grip_bounds.origin.y + grip_bounds.size.height / 2.0,
        handle.origin.y + handle.size.height / 2.0,
    );
}

/// **Nothing on this surface paints anything, unless the grip is asked for.**
/// Measured rather than asserted from the source, because it is the claim the
/// whole `--theme` axis rests on: three anchors, both themes, no background,
/// no border, no radius and no text.
#[gpui::test]
fn without_the_grip_the_surface_has_no_colour_at_all(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for theme in ["dark", "light"] {
        let records = measure(cx, cell(&["--theme", theme]));
        for id in ["resize-group", SIDEBAR, HANDLE, CONTENT] {
            let anchor = find(&records, id);
            assert_eq!(anchor.background, Paint::None, "{id} in {theme}");
            assert_px(anchor.border_width, px(0.0));
            assert_px(anchor.radius, px(0.0));
            assert!(anchor.text.is_none(), "{id} in {theme}");
            assert!(anchor.visible, "{id} in {theme}");
            // Which is also why both declaration lists are empty: v1.6
            // requires a `font` on any anchor that declares it.
            assert!(!anchor.content_sized && !anchor.line_sized, "{id}");
        }
    }

    assert!(resizable::CONTENT_SIZED.is_empty());
    assert!(resizable::LINE_SIZED.is_empty());
}

/// **The two interaction flags move no compared field**, which is this
/// surface's headline and is measured rather than argued. `hover` paints the
/// `::after`, which carries no anchor; `focus` paints a ring, which is a
/// box-shadow `ANCHORS.md` §6 has no field for. Every anchored record is
/// byte-identical to the resting one in both cells.
#[gpui::test]
fn neither_interaction_flag_moves_a_field_the_differ_can_see(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let resting = measure(cx, cell(&[]));

    for flags in ["hover", "focus", "hover,focus"] {
        let driven = measure(cx, cell(&["--flags", flags]));
        for id in ["resize-group", SIDEBAR, HANDLE, CONTENT] {
            let before = find(&resting, id);
            let after = find(&driven, id);
            assert_eq!(before.bounds, after.bounds, "{id} under {flags}");
            assert_eq!(before.background, after.background, "{id} under {flags}");
            assert_eq!(before.radius, after.radius, "{id} under {flags}");
            assert_eq!(
                before.border_width, after.border_width,
                "{id} under {flags}",
            );
            assert_eq!(before.visible, after.visible, "{id} under {flags}");
        }
    }
}

/// Light and dark are the same layout, and — without the grip — the same
/// palette too, because there is no palette. A geometry delta between the two
/// themes would be a bug in the component; a *colour* delta only exists once
/// `--with-handle` puts one on screen.
#[gpui::test]
fn the_theme_changes_only_the_grip(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let dark = measure(cx, cell(&["--theme", "dark", "--with-handle"]));
    let light = measure(cx, cell(&["--theme", "light", "--with-handle"]));

    for id in [
        "resize-group",
        SIDEBAR,
        HANDLE,
        CONTENT,
        "resize-handle-grip",
    ] {
        assert_eq!(at(&dark, id), at(&light, id), "{id}");
    }
    assert_eq!(
        find(&dark, "resize-handle-grip").background,
        Paint::Solid(Theme::DARK.border.value()),
    );
    assert_eq!(
        find(&light, "resize-handle-grip").background,
        Paint::Solid(Theme::LIGHT.border.value()),
    );
    assert_ne!(
        find(&dark, "resize-handle-grip").background,
        find(&light, "resize-handle-grip").background,
    );
}
