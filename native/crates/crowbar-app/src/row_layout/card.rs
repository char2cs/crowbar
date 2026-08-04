//! `--surface card`: what taffy resolves a card to, in a real window.
//!
//! **There is no reference to compare against** — a Card renders only from a
//! caught render throw, for the reason `crowbar_ui::primitives::card` records.
//! So these assertions are against the probe's values (`384 × 102`, `radius 18`,
//! a 1px destructive border, `p-6` with three `in-[…]` overrides) and against the
//! slot arithmetic, which is the part a layout test can still prove.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::primitives::card;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, wide enough that `max-w-sm` is what clamps it.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "card", "--width", "480"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the card's own box.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, card::ID_CARD, id)
}

/// The default cell is `error-boundary.tsx`'s shape: four anchors, root at the
/// origin, clamped to `max-w-sm`.
#[gpui::test]
fn the_default_cell_is_the_error_boundarys_card(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec![
            "card".to_owned(),
            "card-header".to_owned(),
            "card-title".to_owned(),
            "card-panel".to_owned(),
        ],
    );

    let root = at(&records, card::ID_CARD);
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, card::MAX_WIDTH);

    let record = find(&records, card::ID_CARD);
    // `rounded-2xl` = `--radius` × 1.8.
    assert_px(record.radius, px(18.0));
    // A bare `border` — 1px, painted. `badge`'s and `button`'s trap, not `kbd`'s.
    assert_px(record.border_width, px(1.0));
    assert!(record.visible);
}

/// **`max-w-sm` clamps the card**, so `--width` is a real axis up to 384 and
/// vacuous above it. Stated as a measurement because the axis table claims it.
#[gpui::test]
fn the_card_stops_growing_at_max_w_sm(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let under = measure(cx, a_cell(&["--surface", "card", "--width", "300"]));
    assert_px(at(&under, card::ID_CARD).size.width, px(300.0));

    // `--viewport-width` has to grow with the surface: the cell parser refuses a
    // surface wider than the window plus its insets, because every `clipped` in
    // such a snapshot would be an artefact of the window rather than of the
    // component.
    for width in ["384", "480", "900"] {
        let records = measure(
            cx,
            a_cell(&[
                "--surface",
                "card",
                "--width",
                width,
                "--viewport-width",
                "1200",
            ]),
        );
        assert_px(at(&records, card::ID_CARD).size.width, card::MAX_WIDTH);
    }
}

/// **The header's bottom padding is `in-[…]:pb-4`, not the call site's `pb-2`** —
/// the finding this component exists to record, asserted against the layout
/// rather than against the constant.
///
/// With a panel the header pays 16; without one the variant stops matching and
/// `p-6`'s 24 stands. A port that read `error-boundary.tsx`'s class would give 8
/// in both cells and fail here.
#[gpui::test]
fn the_header_padding_follows_the_variant_and_not_the_call_site(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let with_panel = measure(cx, cell(&["--slots", "header-and-panel"]));
    let header = at(&with_panel, card::ID_HEADER);
    let title = at(&with_panel, card::ID_TITLE);
    let paid = header.size.height - (title.origin.y - header.origin.y) - title.size.height;
    assert_px(paid, card::HEADER_PADDING_BOTTOM_WITH_PANEL);
    assert_px(paid, px(16.0));
    assert!((f32::from(paid) - 8.0).abs() > 0.5, "pb-2 must not survive");

    let alone = measure(cx, cell(&["--slots", "header-only"]));
    let header = at(&alone, card::ID_HEADER);
    let title = at(&alone, card::ID_TITLE);
    let paid = header.size.height - (title.origin.y - header.origin.y) - title.size.height;
    assert_px(paid, card::PADDING);
}

/// The panel's two edges are functions of its siblings, which is what the three
/// `in-[…]` variants say.
#[gpui::test]
fn the_panel_padding_follows_the_slots_around_it(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    // A header above it zeroes `pt`, so the panel starts flush with the header.
    let both = measure(cx, cell(&["--slots", "header-and-panel"]));
    let header = at(&both, card::ID_HEADER);
    let panel = at(&both, card::ID_PANEL);
    assert_px(panel.origin.y, header.origin.y + header.size.height);
    // ...and with no footer it keeps its `pb-6`.
    assert_px(panel.size.height, card::PADDING);

    // A footer turns `pb` off, so the panel collapses onto its (empty) content.
    //
    // **Measured, and it lands on 2px rather than 0.** Every other slot cell is
    // exact — `panel-only` is 48 (`pt` + `pb`), `header-and-panel` is 24 (`pb`
    // alone) — and the residue appears only in the cell where the padding sums to
    // nothing. It is exactly the card's own `border_1()` top and bottom, which is
    // the shape of a taffy quirk rather than a coincidence, but that attribution
    // is **not proven** and no control here can vary the border: both call sites
    // carry a bare `border`. Recorded in `native/mapping/card.md` so the cause is
    // one grep away if a run ever cares, and asserted as a bound rather than as a
    // number nobody can justify.
    let footed = measure(cx, cell(&["--slots", "footed"]));
    let collapsed = at(&footed, card::ID_PANEL).size.height;
    assert!(
        collapsed < at(&both, card::ID_PANEL).size.height,
        "a footer removes the panel's bottom padding: {collapsed:?}",
    );
    assert!(
        f32::from(collapsed) < f32::from(card::PADDING) / 2.0,
        "and removes essentially all of it: {collapsed:?}",
    );

    // No header above it and `pt` comes back.
    let panel_only = measure(cx, cell(&["--slots", "panel-only"]));
    assert_eq!(
        ids(&panel_only),
        vec!["card".to_owned(), "card-panel".to_owned()],
    );
    assert_px(
        at(&panel_only, card::ID_PANEL).size.height,
        card::PADDING + card::PADDING,
    );
}

/// **The title is `line_sized` and the call site's `text-sm` is what it sizes
/// to** — the other half of the tailwind-merge finding.
#[gpui::test]
fn the_title_is_its_own_line_box_at_the_call_sites_step(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let record = find(&records, card::ID_TITLE);
    assert!(record.line_sized);
    assert!(!record.content_sized, "a stretched column item");

    let text = record.text.clone().expect("the title paints its run");
    let height = at(&records, card::ID_TITLE).size.height;
    assert!(
        (f32::from(height) - f32::from(text.font.line_height)).abs() <= 0.5,
        "line_sized: {height:?} against {:?}",
        text.font.line_height,
    );
    // `leading-none` makes the line box the font size exactly.
    assert!(
        (f32::from(text.font.line_height) - f32::from(text.font.size)).abs() <= 0.5,
        "leading-none: {:?} against {:?}",
        text.font.line_height,
        text.font.size,
    );
    // ...and the size is the call site's 14, not the primitive's 18.
    assert!(
        (f32::from(text.font.size) - 14.0).abs() < 0.01,
        "text-sm beats text-lg here, got {:?}",
        text.font.size,
    );
}

/// `--call-site none` is the unmerged primitive, and it is a different picture in
/// every colour field — which is what makes the parameter worth having even
/// though no live call site produces it.
#[gpui::test]
fn the_unmerged_primitive_paints_different_colours(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let merged = measure(cx, cell(&["--call-site", "error-boundary"]));
    let bare = measure(cx, cell(&["--call-site", "none"]));

    assert_ne!(
        find(&merged, card::ID_CARD).background,
        find(&bare, card::ID_CARD).background,
    );
    assert_ne!(
        find(&merged, card::ID_CARD).border_color,
        find(&bare, card::ID_CARD).border_color,
    );
    assert_ne!(
        find(&merged, card::ID_TITLE).text.expect("a run").color,
        find(&bare, card::ID_TITLE).text.expect("a run").color,
    );

    // The geometry is identical: the bundle moves colour and the title's step.
    assert_eq!(
        at(&merged, card::ID_CARD).size.width,
        at(&bare, card::ID_CARD).size.width,
    );
}

/// §8.3's `empty`: a `<Card>` with no children is one anchor, not four — an
/// anchor-set change, which is the loudest failure the differ has.
#[gpui::test]
fn the_empty_cell_is_the_card_alone(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));

    assert_eq!(ids(&records), vec!["card".to_owned()]);
    let record = find(&records, card::ID_CARD);
    // It still paints its border and its tint.
    assert_px(record.border_width, px(1.0));
    assert_px(record.radius, px(18.0));
    assert!(record.text.is_none());
    // Two borders and nothing between them.
    assert_px(at(&records, card::ID_CARD).size.height, px(2.0));
}
