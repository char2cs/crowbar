//! `--surface tooltip`, laid out in a real window.
//!
//! Written against the live reference — `/tmp/p3-ref-tooltip.json`, the
//! `Close ⌘W` fixture (`tab-bar-item.tsx`'s close button, opened live through
//! `HTMLElement.focus()`): a `99.296875 × 32` box at `data-side="bottom"`,
//! `rounded-lg`, a 1px `border-border/70` border, `bg-card/95`.
//!
//! Unlike `popover`, this surface renders on the **first** frame: it is a
//! plain `div()` tree with no vendor deferred/anchored positioning behind it
//! (see `crowbar_ui::primitives::tooltip`'s module docs for why — the build
//! verdict means there is no `gpui_component::Popover`-style trigger-bound
//! capture to wait on). `super::measure`'s shared `on_settled_frame` signal
//! still applies and still passes; it simply settles on the first draw.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::tooltip;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "tooltip"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to this surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, tooltip::ID_TOOLTIP, id)
}

/// **The default cell's height is the live fixture's `32px`, exactly** — the
/// width is asserted as its **composition**, not against the live reference's
/// `99.296875`, for `kbd.rs`'s reason: `main.rs` calls `add_fonts` before
/// opening its window and this harness does not, so a headless `gpui::test`
/// shapes runs with a system fallback face and its advance widths are its
/// own. The height does not have this problem — it is padding and border
/// around a line box in whole pixels, not a shaped advance.
#[gpui::test]
fn the_default_cells_height_is_the_live_fixtures_thirty_two_pixels(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = at(&records, tooltip::ID_TOOLTIP);

    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.height, px(32.0));

    // The composition: two borders, two paddings, the run, the gap, the chip.
    let run = find(&records, tooltip::ID_TOOLTIP)
        .text
        .expect("the root paints a run")
        .width;
    let chip = find(&records, tooltip::ID_SHORTCUT)
        .text
        .expect("the chip paints a run")
        .width;
    let chip_width = f32::from(chip.ceil())
        + f32::from(tooltip::SHORTCUT_PADDING_X) * 2.0
        + f32::from(tooltip::BORDER_WIDTH) * 2.0; // the chip's own two borders
    let expected = f32::from(tooltip::BORDER_WIDTH) * 2.0
        + f32::from(tooltip::PADDING_X) * 2.0
        + f32::from(run.ceil())
        + f32::from(tooltip::GAP)
        + chip_width;
    assert_px(root.size.width, px(expected));
}

/// **The root's own paint**: `rounded-lg`, a real 1px border at
/// `border-border/70`, `bg-card/95`.
#[gpui::test]
fn the_root_paints_the_measured_chrome(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let root = find(&records, tooltip::ID_TOOLTIP);

    assert_px(root.radius, tooltip::RADIUS);
    assert_px(root.radius, px(10.0));
    assert_px(root.border_width, tooltip::BORDER_WIDTH);
    assert_px(root.border_width, px(1.0));
    assert_eq!(
        root.background,
        Paint::Solid(
            Theme::DARK
                .card
                .mix(
                    tooltip::BACKGROUND_ALPHA,
                    crowbar_ui::theme::Color::TRANSPARENT
                )
                .value()
        ),
    );
    assert_eq!(
        root.border_color,
        Some(
            Theme::DARK
                .border
                .mix(tooltip::BORDER_ALPHA, crowbar_ui::theme::Color::TRANSPARENT)
                .value()
        ),
    );
    assert!(root.visible);
    assert_eq!(
        root.text.as_ref().map(|t| t.content.to_string()),
        Some("Close".to_owned())
    );
}

/// **The shortcut chip carries its own anchor**, sized to its own content and
/// coloured from `theme.card`/`theme.border` at full alpha — not mixed, unlike
/// the root.
#[gpui::test]
fn the_shortcut_chip_is_its_own_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let chip = find(&records, tooltip::ID_SHORTCUT);

    assert_px(chip.radius, tooltip::SHORTCUT_RADIUS);
    assert_px(chip.radius, px(8.0));
    assert_eq!(chip.background, Paint::Solid(Theme::DARK.card.value()));
    assert_eq!(chip.border_color, Some(Theme::DARK.border.value()));
    assert_eq!(
        chip.text.as_ref().map(|t| t.content.to_string()),
        Some("⌘W".to_owned()),
    );

    let seen = ids(&records);
    assert!(seen.contains(&tooltip::ID_TOOLTIP.to_owned()));
    assert!(seen.contains(&tooltip::ID_SHORTCUT.to_owned()));
}

/// **`--no-shortcut` reaches the path-breadcrumb fixture's shape**: no chip
/// anchor at all, and the root's height is unchanged — both fixtures measured
/// `32px` tall live. The width is asserted as its composition, for the reason
/// the default cell's test states.
#[gpui::test]
fn no_shortcut_omits_the_chip_and_keeps_the_measured_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--text", "demo/src/a/a.ts", "--no-shortcut"]));

    assert!(!ids(&records).contains(&tooltip::ID_SHORTCUT.to_owned()));
    let root = at(&records, tooltip::ID_TOOLTIP);
    assert_px(root.size.height, px(32.0));

    let run = find(&records, tooltip::ID_TOOLTIP)
        .text
        .expect("the root paints a run")
        .width;
    let expected = f32::from(tooltip::BORDER_WIDTH) * 2.0
        + f32::from(tooltip::PADDING_X) * 2.0
        + f32::from(run.ceil());
    assert_px(root.size.width, px(expected));
}

/// **`empty` collapses the root to its padding and border around nothing** —
/// modelled and unreached, the same shape `popover`'s `empty` flag takes.
#[gpui::test]
fn empty_collapses_the_root_to_its_padding_and_border(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let resting = measure(cx, cell(&["--no-shortcut"]));
    let empty = measure(cx, cell(&["--flags", "empty", "--no-shortcut"]));

    let resting_width = at(&resting, tooltip::ID_TOOLTIP).size.width;
    let empty_width = at(&empty, tooltip::ID_TOOLTIP).size.width;
    assert!(
        f32::from(empty_width) < f32::from(resting_width),
        "empty ({empty_width:?}) should be narrower than resting ({resting_width:?})",
    );
    // The height does not move: it is the line box plus padding and border,
    // and an empty run still occupies a (zero-width) line.
    assert_px(
        at(&empty, tooltip::ID_TOOLTIP).size.height,
        at(&resting, tooltip::ID_TOOLTIP).size.height,
    );
}

/// The light table paints a different chip and root — `--card` and `--border`
/// both move.
#[gpui::test]
fn the_light_table_paints_different_tokens(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let root = find(&records, tooltip::ID_TOOLTIP);

    assert_eq!(
        root.background,
        Paint::Solid(
            Theme::LIGHT
                .card
                .mix(
                    tooltip::BACKGROUND_ALPHA,
                    crowbar_ui::theme::Color::TRANSPARENT
                )
                .value()
        ),
    );
    assert_ne!(Theme::LIGHT.card, Theme::DARK.card);
}
