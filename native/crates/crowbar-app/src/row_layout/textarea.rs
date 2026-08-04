//! `--surface textarea`, laid out in a real window.
//!
//! **There is no live reference for this surface** — `textarea.tsx`'s only
//! importer, `commit-popover.tsx`, sits behind a git panel whose "Changes"
//! list would not populate in this item's dev environment even after a real
//! `git/stage` API call and a full reload, confirmed against the backend's
//! own `git/status` response showing six real dirty files. See
//! `crowbar_ui::primitives::textarea` and `native/mapping/textarea.md`. What
//! this file establishes instead is the arithmetic against `textarea.tsx`'s
//! own compiled classes, measured by injecting them into the live app's DOM
//! — the same values `crowbar_ui::primitives::textarea`'s constants carry,
//! laid out here in a real window rather than only unit-tested against the
//! theme.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::textarea;
use crowbar_ui::primitives::textarea::ALL_SIZES;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied. `--width` is
/// pinned wide enough (400px) that `w_full()` never binds against
/// `RowSurface`'s own inset arithmetic in a way that would be mistaken for
/// this component's.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "textarea", "--width", "400"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to this surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, textarea::ID_CONTROL, id)
}

/// **The resting cell**: the live call site's own shape — control `80px`
/// tall (its own `min-h-20`, taller than the field's floor), field
/// stretched to `78px` (`80 − 2×border`).
///
/// The field's own `w_full()` resolves against the control's **content**
/// box, not its border box — `398px` inside a `400px` control with a 1px
/// border each side, `input.rs`'s own control/field inset, confirmed
/// independently here.
#[gpui::test]
fn the_resting_cell_is_the_live_call_sites_shape(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = at(&records, textarea::ID_CONTROL);
    let field = at(&records, textarea::ID_FIELD);

    assert_px(control.origin.x, px(0.0));
    assert_px(control.origin.y, px(0.0));
    assert_px(control.size.width, px(400.0));
    assert_px(control.size.height, px(80.0));

    assert_px(field.size.width, px(400.0) - textarea::BORDER_WIDTH * 2.0);
    assert_px(field.size.height, px(78.0));

    let record = find(&records, textarea::ID_CONTROL);
    assert_px(record.border_width, textarea::BORDER_WIDTH);
    assert_px(record.radius, Theme::DARK.radius_lg.value());
}

/// **`--bare` drops the call site's `min-h-20`, and the field falls back to
/// its own `min-h-17.5` floor (70px), the control to `72px`.**
#[gpui::test]
fn bare_drops_the_call_sites_min_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let commit_message = measure(cx, cell(&[]));
    let bare = measure(cx, cell(&["--bare"]));

    let commit_field = at(&commit_message, textarea::ID_FIELD);
    let bare_field = at(&bare, textarea::ID_FIELD);
    assert_px(commit_field.size.height, px(78.0));
    assert_px(bare_field.size.height, px(70.0));
    assert!(commit_field.size.height > bare_field.size.height);

    let bare_control = at(&bare, textarea::ID_CONTROL);
    assert_px(
        bare_control.size.height,
        bare_field.size.height + textarea::BORDER_WIDTH * 2.0,
    );
}

/// **Every `Size` arm's own floor**, laid out in a real window under
/// `--bare` (so the call site's taller `min-h-20` cannot mask a smaller
/// arm's own number).
#[gpui::test]
fn every_size_arms_own_floor_is_measured(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut heights = Vec::new();
    for size in ALL_SIZES {
        let records = measure(cx, cell(&["--bare", "--size", size.name()]));
        let field = at(&records, textarea::ID_FIELD);
        heights.push((size, field.size.height));
    }

    assert_px(heights[0].1, size_min_height(ALL_SIZES[0]));
    for pair in heights.windows(2) {
        assert!(pair[0].1 < pair[1].1, "{:?} vs {:?}", pair[0], pair[1]);
    }
}

fn size_min_height(size: textarea::Size) -> Pixels {
    size.min_height()
}

/// The field's radius is the control's, read from the same token.
#[gpui::test]
fn the_field_inherits_the_controls_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = find(&records, textarea::ID_CONTROL);
    let field = find(&records, textarea::ID_FIELD);
    assert_px(control.radius, Theme::DARK.radius_lg.value());
    assert_px(field.radius, control.radius);
}

/// **The control's background is `dark:bg-input/32`**, the identical alpha
/// `input.rs`'s own `DARK_BACKGROUND_ALPHA` names — confirmed independently
/// here rather than assumed from the shared substring.
#[gpui::test]
fn the_controls_background_is_input_mixed_at_32_percent(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let control = find(&records, textarea::ID_CONTROL);

    assert_eq!(
        control.background,
        Paint::Solid(
            Theme::DARK
                .input
                .mix(
                    textarea::DARK_BACKGROUND_ALPHA,
                    crowbar_ui::Color::TRANSPARENT
                )
                .value()
        ),
    );
    assert_eq!(control.border_color, Some(Theme::DARK.input.value()));
}

/// `focus` moves the control's border colour to `theme.ring`, and `invalid`
/// stacked over it wins — `input.rs`'s own chain, confirmed independently on
/// the byte-identical rules.
#[gpui::test]
fn focus_and_invalid_move_the_controls_border_colour(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&[]));
    assert_eq!(
        find(&resting, textarea::ID_CONTROL).border_color,
        Some(Theme::DARK.input.value()),
    );

    let focused = measure(cx, cell(&["--flags", "focus"]));
    assert_eq!(
        find(&focused, textarea::ID_CONTROL).border_color,
        Some(Theme::DARK.ring.value()),
    );

    let invalid = measure(cx, cell(&["--invalid"]));
    assert_eq!(
        find(&invalid, textarea::ID_CONTROL).border_color,
        Some(
            Theme::DARK
                .destructive
                .mix(
                    textarea::INVALID_BORDER_ALPHA,
                    crowbar_ui::Color::TRANSPARENT
                )
                .value()
        ),
    );

    let both = measure(cx, cell(&["--invalid", "--flags", "focus"]));
    assert_eq!(
        find(&both, textarea::ID_CONTROL).border_color,
        Some(
            Theme::DARK
                .destructive
                .mix(
                    textarea::INVALID_FOCUS_BORDER_ALPHA,
                    crowbar_ui::Color::TRANSPARENT
                )
                .value()
        ),
    );
}

/// The field paints no text, at any size or call-site arm.
#[gpui::test]
fn no_anchor_on_this_surface_reports_text(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [vec![], vec!["--bare"], vec!["--size", "lg"]] {
        let records = measure(cx, cell(&line));
        for record in &records {
            assert!(record.text.is_none(), "{line:?}: {} paints text", record.id);
            assert!(!record.content_sized, "{line:?}: {}", record.id);
            assert!(!record.line_sized, "{line:?}: {}", record.id);
        }
    }
}

/// The anchor set is fixed at two, on every cell.
#[gpui::test]
fn the_anchor_set_is_two_on_every_cell(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for args in [
        vec![],
        vec!["--bare"],
        vec!["--disabled"],
        vec!["--invalid"],
        vec!["--flags", "focus"],
        vec!["--size", "sm"],
    ] {
        let seen = ids(&measure(cx, cell(&args)));
        assert_eq!(seen.len(), 2, "{args:?}: {seen:?}");
        assert!(seen.contains(&textarea::ID_CONTROL.to_owned()), "{args:?}");
        assert!(seen.contains(&textarea::ID_FIELD.to_owned()), "{args:?}");
    }
}
