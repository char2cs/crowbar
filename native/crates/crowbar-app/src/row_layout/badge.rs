//! `--surface badge`: what taffy resolves the badge's box to, in a real window.
//!
//! Carried no module docs when it landed as an inline `mod` block; this header
//! is the file's, and nothing below it changed in the move. What the module is
//! about is one box and the two numbers on it — the height that `tailwind-merge`
//! decides and the width that is content-sized rather than authored.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::badge;
use crowbar_ui::primitives::badge::{ALL_CALL_SITES, ALL_SIZES, ALL_VARIANTS, Size};
use crowbar_ui::theme::Color;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "badge"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the badge itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, badge::ID_BADGE, id)
}

/// `sm:min-w-4.5` on the fixture's `size="default"`: the floor a short label
/// is measured against.
///
/// A module constant rather than a local, because `leak_checked!` has to be
/// a test's **first statement** (`check-invariants.sh` rule 6) and clippy's
/// `items_after_statements` then forbids a `const` below it.
const FLOOR: Pixels = px(18.0);

/// **The default cell is the live `agent` badge**, and the two numbers that
/// matter are its height and its radius.
///
/// 18 is `sm:h-4.5` — the *variant's* — even though the call site writes
/// `h-4`, which reads as 16. That is the tailwind-merge trap of the whole
/// component, and the live element reports exactly 18.
///
/// The width is deliberately **not** asserted against 44.34: it is the
/// shaped advance of "agent" plus the padding and the border, and the two
/// engines shape independently. What is asserted is that it is content-
/// sized, which is what tells the differ to compare against `ceil(44.34)`.
#[gpui::test]
fn the_default_cell_is_the_live_agent_badge(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let seen = ids(&records);

    assert_eq!(seen, vec!["badge".to_owned()]);
    let subject = at(&records, "badge");
    assert_px(subject.origin.x, px(0.0));
    assert_px(subject.origin.y, px(0.0));
    assert_px(subject.size.height, px(18.0));

    let record = find(&records, "badge");
    assert_px(record.radius, px(6.0));
    assert_px(record.border_width, px(1.0));
    assert!(record.visible);
    // And it is drawn at the ordinary inset, so the root-relative
    // subtraction in the snapshot is doing work on both axes.
    assert_px(record.bounds.origin.x, px(24.0));

    // The run the reference reports as `text: "agent"`.
    let text = record.text.clone().expect("the badge paints its label");
    assert_eq!(text.content, "agent");
    assert!(!text.clipped, "whitespace-nowrap with no overflow-hidden");
    // v1.2: `font.family` is the **declared** first family, and an inherited
    // `.SystemUIFont` is a string the DOM will never produce. The reference
    // says `CalSansUI`, on a 12px face at 500.
    assert_eq!(text.font.family, "CalSansUI");
    assert_px(text.font.size, px(12.0));
    assert!((text.font.weight - 500.0).abs() < f32::EPSILON);
}

/// **v1.5 reaches the extractor**, which is the field this surface's only
/// anchor rests on: without it the differ compares 45 against 44.34 and
/// reports a 0.66px delta that is entirely gpui's `ceil`.
///
/// Read off the recorded anchor rather than off `CONTENT_SIZED`, because a
/// declaration that never reached the sink would leave the constant right
/// and the snapshot wrong.
#[gpui::test]
fn the_badge_declares_content_sized_and_not_line_sized(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let record = find(&records, "badge");

    assert!(record.content_sized, "ANCHORS.md v1.5");
    assert!(
        !record.line_sized,
        "ANCHORS.md v1.6 — the height is authored"
    );

    // And the reason: the box is not the line box. The live pair is 18
    // against 16, which is well outside the ±0.5 v1.6 would compare at.
    let line = record
        .text
        .clone()
        .expect("the badge paints its label")
        .font
        .line_height;
    assert!(
        (record.bounds.size.height - line).abs() > px(0.5),
        "box {:?} against line {line:?}",
        record.bounds.size.height,
    );
}

/// The width **is** the label's advance plus the padding and the border, on
/// every content length — which is what "content-sized" means, stated as a
/// measurement rather than as a declaration.
///
/// `min-w-*` is a **floor**, so the box is the larger of the two — and the
/// one-character label lands on it **exactly**, which was measured rather
/// than arranged: `ceil("3") = 8`, plus 4px of padding either side and the
/// two border pixels, is 18, and `sm:min-w-4.5` is 18. The floor being the
/// same `--spacing` step as the height is what makes a one-character badge a
/// disc, and here it is that to the pixel.
#[gpui::test]
fn the_width_is_the_labels_advance_against_the_floor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut widths = Vec::new();
    for content in ["short", "normal", "overflow"] {
        let records = measure(cx, cell(&["--content", content]));
        let record = find(&records, "badge");
        let text = record.text.clone().expect("a label");
        // gpui ceils a run's max-content width, which is exactly what v1.5
        // makes the differ compare against on the other side.
        let padded = text.width.ceil() + px(4.0) * 2.0 + px(1.0) * 2.0;

        assert_px(record.bounds.size.width, padded.max(FLOOR));
        if content == "short" {
            assert_px(padded, FLOOR);
            assert_px(record.bounds.size.width, record.bounds.size.height);
        } else {
            assert!(padded > FLOOR, "{content}: {padded:?}");
        }
        widths.push(f32::from(record.bounds.size.width));
    }

    // The three lengths are three different pictures, or `--content` would
    // be decoration on the one axis it can move.
    assert!(widths[0] < widths[1] && widths[1] < widths[2], "{widths:?}");
}

/// **`border.w` is 1 on every variant** — the field `ANCHORS.md` v1.1
/// compares exactly, and the one a "no visible border" shortcut gets wrong
/// on every cell.
///
/// Read off the extractor rather than off the constant: a `.border_1()` that
/// never reached the style would leave `BORDER_WIDTH` equal to 1 anyway.
#[gpui::test]
fn every_variant_reports_a_one_pixel_border(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for variant in ALL_VARIANTS {
        // `--call-site none` so the variant's own border colour is what
        // paints; the default cell's bundle writes `border-primary/30` over
        // the top of it.
        let records = measure(
            cx,
            cell(&["--variant", variant.name(), "--call-site", "none"]),
        );
        let record = find(&records, "badge");

        assert_px(record.border_width, px(1.0));
        assert!(record.border_color.is_some(), "{}", variant.name());
        // Every variant paints a background, unlike `button`'s `link`.
        assert!(
            matches!(record.background, Paint::Solid(_)),
            "{}: {:?}",
            variant.name(),
            record.background,
        );
    }
}

/// Every size's box, at both breakpoints, against the compiled
/// `calc(var(--spacing) * n)` — and the radius the `sm` size swaps in.
///
/// `--call-site none` throughout, because this test is about the *variant's*
/// own classes and the default cell has a bundle merged over them.
#[gpui::test]
fn every_size_resolves_to_its_compiled_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    for size in ALL_SIZES {
        for (viewport, breakpoint) in [
            ("800", crowbar_ui::surfaces::rows::Breakpoint::Sm),
            ("600", crowbar_ui::surfaces::rows::Breakpoint::Base),
        ] {
            let records = measure(
                cx,
                cell(&[
                    "--size",
                    size.name(),
                    "--call-site",
                    "none",
                    "--viewport-width",
                    viewport,
                    "--width",
                    "300",
                ]),
            );
            let record = find(&records, "badge");

            assert_px(record.bounds.size.height, size.height(breakpoint));
            assert_px(record.radius, size.radius(&theme));
            // The floor is the same `--spacing` step as the height, so an
            // empty badge of this size would be a circle.
            assert_px(size.min_width(breakpoint), size.height(breakpoint));
        }
    }

    // The `sm` size's `rounded-[.25rem]` is 4, which is *not* on the token
    // scale — `--radius-sm` is 6. A port that read the token would be two
    // pixels out on the Phase 1 gate's own badge.
    let records = measure(cx, cell(&["--size", "sm", "--call-site", "none"]));
    assert_px(find(&records, "badge").radius, px(4.0));
    assert_ne!(Size::Sm.radius(&theme), theme.radius_sm.value());
}

/// The call-site trap, measured on both sides of the breakpoint: an
/// unprefixed `h-4` is the whole height below 640px and is dead above it.
#[gpui::test]
fn a_call_sites_height_is_dead_above_the_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let wide = measure(cx, cell(&["--viewport-width", "800"]));
    assert_px(find(&wide, "badge").bounds.size.height, px(18.0));

    let narrow = measure(cx, cell(&["--viewport-width", "600"]));
    assert_px(find(&narrow, "badge").bounds.size.height, px(16.0));

    // Without the bundle the variant's own unprefixed height stands.
    let bare = measure(
        cx,
        cell(&["--call-site", "none", "--viewport-width", "600"]),
    );
    assert_px(find(&bare, "badge").bounds.size.height, px(22.0));
}

/// The call site's colours reach the extractor, and they beat the variant's.
///
/// `border-primary/30` over `outline`'s `border-input` is the pair the
/// reference reports as `#516a364d`, and it is the one place a bundle moves
/// a *compared* field rather than a geometry one.
#[gpui::test]
fn a_call_sites_colours_beat_the_variants(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    let agent = measure(cx, cell(&["--call-site", "agent"]));
    let bare = measure(cx, cell(&["--call-site", "none"]));

    let merged = find(&agent, "badge");
    let plain = find(&bare, "badge");
    assert_ne!(merged.border_color, plain.border_color);
    assert_eq!(
        merged.border_color,
        Some(theme.primary.mix(30.0, Color::TRANSPARENT).value()),
    );
    assert_eq!(plain.border_color, Some(theme.input.value()));

    // And the label's colour, which is the anchor's `fg`.
    let fg = |records: &[RawAnchor]| find(records, "badge").text.clone().expect("a label").color;
    assert_ne!(fg(&agent), fg(&bare));
    assert_eq!(fg(&agent), theme.primary.value());

    // The three bundles are three pictures, or `--call-site` is decoration.
    let borders: Vec<_> = ALL_CALL_SITES
        .into_iter()
        .map(|site| {
            let records = measure(cx, cell(&["--call-site", site.name()]));
            find(&records, "badge").border_color
        })
        .collect();
    assert_eq!(borders.len(), 3);
    assert_ne!(borders[0], borders[1]);
    assert_ne!(borders[1], borders[2]);
}

/// `empty` is a real picture: no label, so the box falls to `min-w-*` and
/// the badge is a circle. And the anchor stops paying the text group.
#[gpui::test]
fn an_empty_badge_is_a_circle_of_its_floor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let record = find(&records, "badge");

    assert_px(record.bounds.size.width, px(18.0));
    assert_px(record.bounds.size.height, px(18.0));
    assert!(record.text.is_none(), "no label, no text group");
    // Still content-sized: the declaration is a property of the component,
    // and `ceil` of an integral floor is the floor.
    assert!(record.content_sized);
}

/// The interaction flags move `bg` **only** when the `render` prop made the
/// badge a button or an anchor — which no live call site does.
///
/// Two assertions, and the second is the one that would catch a port that
/// dropped the `[button&,a&]:` qualifier: hovering a `<span>` has to be
/// byte-identical to resting.
#[gpui::test]
fn hover_moves_nothing_until_the_badge_is_a_button(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = find(&measure(cx, cell(&[])), "badge");
    let hovered_span = find(&measure(cx, cell(&["--flags", "hover"])), "badge");
    let hovered_button = find(
        &measure(cx, cell(&["--flags", "hover", "--interactive"])),
        "badge",
    );

    assert_eq!(hovered_span.background, resting.background);
    assert_ne!(hovered_button.background, resting.background);

    // `focus` paints a ring, which is a box-shadow, which §6 has no field
    // for — so the cell is identical to resting on every compared field.
    let focused = find(&measure(cx, cell(&["--flags", "focus"])), "badge");
    assert_eq!(focused.background, resting.background);
    assert_eq!(focused.bounds.size, resting.bounds.size);
    assert_eq!(focused.border_width, resting.border_width);
}

/// A glyph is an empty box that widens the badge by its own extent plus the
/// `gap-1` — and it carries no anchor, so it changes the *root's* geometry
/// and nothing else in the snapshot.
#[gpui::test]
fn a_glyph_widens_the_badge_by_its_box_and_the_gap(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let plain = find(&measure(cx, cell(&[])), "badge");
    let with_glyph = find(&measure(cx, cell(&["--glyph"])), "badge");

    assert_eq!(
        ids(&measure(cx, cell(&["--glyph"]))),
        vec!["badge".to_owned()]
    );
    assert_px(
        with_glyph.bounds.size.width,
        plain.bounds.size.width
            + Size::Default.glyph(crowbar_ui::surfaces::rows::Breakpoint::Sm)
            + px(4.0),
    );
}
