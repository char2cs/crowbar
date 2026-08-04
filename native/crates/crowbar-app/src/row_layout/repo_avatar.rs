//! `--surface repo-avatar`: what taffy resolves the three pictures to, in a
//! real window.
//!
//! `repo-avatar.tsx` renders exactly one element per call — no persistent
//! wrapper the way `avatar.rs`'s root is one — so every cell here carries
//! exactly one anchor, `repo-avatar`, whatever picture it holds.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::repo_avatar::{self, ALL_SIZES};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "repo-avatar"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the avatar itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, repo_avatar::ID, id)
}

/// **The default cell is the letter fallback at `sm`**: `16 × 16`,
/// `radius_sm` (6px, not `rounded-full`), `font-bold`, an `"RE"` run.
///
/// `"RE"`, not `"R"`: `--content` defaults to `ContentLength::Normal`, the
/// shared convention every `--content` surface uses (`git_status_row.rs`),
/// not a repo-avatar-specific choice — the bare cell is
/// `label_of(ContentLength::Normal)`, and `label_of`'s own match arm names
/// it `"RE"`. `RepoAvatar::fixture`'s hardcoded `"R"` is not what this cell
/// shows: `Params::repo_avatar` overrides `label` from `cell.content`
/// unconditionally (only `--flags empty` beats it), so the fixture's literal
/// string never survives past a bare `--surface repo-avatar`.
///
/// The one-line mutation that turns this red: swap `theme.radius_sm.value()`
/// for `FULL_RADIUS` in `RepoAvatar::letter_box` — `record.radius` would then
/// read `f32::MAX` against this test's `px(6.0)`.
#[gpui::test]
fn the_default_cell_is_the_letter_fallback(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(ids(&records), vec!["repo-avatar".to_owned()]);

    let root = at(&records, "repo-avatar");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let theme = Theme::DARK;
    let record = find(&records, "repo-avatar");
    assert_eq!(record.radius, theme.radius_sm.value());
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert!(matches!(record.background, Paint::Solid(_)));

    let text = record.text.clone().expect("the letter fallback paints text");
    assert_eq!(text.content, "RE");
    assert_px(text.font.size, px(10.0)); // text-[10px], sm
    assert!((text.font.weight - 700.0).abs() < f32::EPSILON, "font-bold");
}

/// **`rounded-sm`, not `rounded-full`.** `avatar.tsx`'s fallback is a circle;
/// this one is a small rounded square — a real shape difference between the
/// two ports, not an oversight.
///
/// The one-line mutation: change `.rounded(theme.radius_sm.value())` to
/// `.rounded(crowbar_ui::components::avatar::FULL_RADIUS)` in
/// `RepoAvatar::letter_box` — this test's `assert_ne!` would then read
/// `f32::MAX == f32::MAX` and fail.
#[gpui::test]
fn the_letter_shape_is_a_rounded_square_not_a_circle(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let theme = Theme::DARK;
    let record = find(&records, "repo-avatar");
    assert_eq!(record.radius, theme.radius_sm.value());
    // `Pixels`, not the raw `f32` either side would convert to — the same
    // typed comparison line 74 above already makes, and the one `avatar.rs`'s
    // own `assert_eq!(FULL_RADIUS, px(f32::MAX))` makes too. Comparing the
    // laid-out `Pixels` value directly is what avoids `clippy::float_cmp`
    // here; converting both sides to `f32` first, the way this line used to,
    // is what triggered it.
    assert_ne!(record.radius, crowbar_ui::components::avatar::FULL_RADIUS);
}

/// Every size resolves to its own authored box — `16 × 16`, `20 × 20`,
/// `24 × 24` — and to nothing else. There is no `md` between `sm` and `lg`.
#[gpui::test]
fn every_size_resolves_to_its_own_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    assert_eq!(ALL_SIZES.len(), 3);

    for (word, extent) in [("sm", 16.0), ("lg", 20.0), ("xl", 24.0)] {
        let records = measure(cx, cell(&["--size", word]));
        let root = at(&records, "repo-avatar");
        assert_px(root.size.width, px(extent));
        assert_px(root.size.height, px(extent));
    }
}

/// **`xl`'s letter fallback paints `font.line_height` at `19.5px`** — `13 ×
/// 1.5`, matching `getComputedStyle` on the live node
/// (`native/mapping/repo-avatar.md`'s VERDICT: `fontSize: 13px, lineHeight:
/// 19.5px` — that is the live app's own number, not the port's). Before this
/// fix, `RepoAvatar::letter_box` set no explicit `line_height` at all, so
/// gpui's own internal default (`phi()`, the golden ratio) leaked through:
/// `13 × φ ≈ 21.03`, snapped to the `21.0` this item's own drive read back
/// **from the port**.
///
/// **Mutation, run:** in `RepoAvatar::letter_box`
/// (`crates/crowbar-ui/src/components/repo_avatar.rs`), changed the guard to
/// `if false && self.size == Size::Xl { … }`, so `cell` is returned
/// unconditionally with no explicit line-height call — reproducing the
/// pre-fix behaviour exactly. `cargo test -p crowbar-app --bin crowbar-app
/// row_layout::repo_avatar::xl_letter_line_height_is_19_5_matching_the_live_capture`
/// failed as predicted, with the real output:
///
/// ```text
/// thread 'row_layout::repo_avatar::xl_letter_line_height_is_19_5_matching_the_live_capture' (277808014) panicked at /private/tmp/crowbar-p364-fixtures/native/crates/crowbar-app/src/row_layout/repo_avatar.rs:138:5:
/// expected 19.5px, got 21px
/// ```
///
/// `21px` is exactly `13 × φ` (gpui's default `phi()` line-height ratio)
/// device-pixel-snapped — the same number the live drive's own delta
/// reported (`font.line_height: 21.0, expected 19.5`). Reverted after
/// confirming.
#[gpui::test]
fn xl_letter_line_height_is_19_5_matching_the_live_capture(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--size", "xl"]));
    let record = find(&records, "repo-avatar");
    let text = record.text.expect("the letter fallback paints text");
    assert_px(text.font.line_height, px(19.5));
}

/// **`sm`/`lg` are deliberately left alone.** This is not a claim that
/// `16.0`/`18.0` (gpui's own `phi()` default, rounded) are *correct* — see
/// `RepoAvatar::letter_box`'s own doc comment for why neither size has an
/// established number to assert instead. This test exists so a future
/// change that starts asserting a specific `sm`/`lg` ratio has to touch
/// this file and explain where the number came from, rather than silently
/// inheriting whatever `xl`'s fix happens to produce.
#[gpui::test]
fn sm_and_lg_are_unchanged_by_the_xl_fix(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for (word, size_px) in [("sm", 10.0_f32), ("lg", 11.0_f32)] {
        let records = measure(cx, cell(&["--size", word]));
        let record = find(&records, "repo-avatar");
        let text = record.text.expect("the letter fallback paints text");
        assert_px(text.font.size, px(size_px));
        // Not `19.5` (the xl ratio) and not the arbitrary phi default
        // asserted as if it were meaningful — just: whatever gpui's own
        // unset default currently resolves to, unasserted beyond "some
        // positive value", because no reference establishes a number here.
        assert!(text.font.line_height > px(0.0));
    }
}

/// **The loaded image is `rounded-sm` too** — unlike `avatar.tsx`'s own
/// image, which carries no radius class at all (`0`). A real, measured
/// difference between the two ports' image boxes.
///
/// The one-line mutation: drop `.rounded(theme.radius_sm.value())` from
/// `RepoAvatar::image_box` — `record.radius` would read `0` against this
/// test's non-zero `theme.radius_sm.value()`.
#[gpui::test]
fn the_loaded_image_is_an_empty_box_with_its_own_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--kind", "image", "--image", "loaded"]));
    let theme = Theme::DARK;

    let root = at(&records, "repo-avatar");
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let record = find(&records, "repo-avatar");
    assert_eq!(record.radius, theme.radius_sm.value());
    assert_px(record.border_width, px(0.0));
    assert_eq!(record.background, Paint::None, "no bitmap is drawn");
    assert!(record.text.is_none(), "an image paints no text");
}

/// **An errored image is the same picture as the letter fallback.** A fact
/// about the source — `RepoAvatarImg`'s `fallback` prop is
/// `repo-avatar.tsx`'s own `letterFallback` value — asserted by comparing the
/// two cells' full recorded pictures, not merely their kind.
#[gpui::test]
fn an_errored_image_is_pixel_identical_to_the_letter_fallback(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let errored = measure(cx, cell(&["--kind", "image", "--image", "errored"]));
    let letter = measure(cx, cell(&["--kind", "letter"]));

    let a = find(&errored, "repo-avatar");
    let b = find(&letter, "repo-avatar");
    assert_eq!(a.bounds.size, b.bounds.size);
    assert_eq!(a.radius, b.radius);
    assert_eq!(a.background, b.background);
    assert_eq!(
        a.text.map(|t| t.content),
        b.text.map(|t| t.content),
        "both show the fixture's own label",
    );
}

/// **The emoji glyph paints no background.** `avatar.color` is not in the
/// emoji branch's class list at all — the one branch of this component
/// `avatar.color` never reaches.
///
/// The one-line mutation: add `.bg(self.background)` to `RepoAvatar::emoji_box`
/// — `record.background` would then read `Paint::Solid(_)` against this
/// test's `Paint::None`.
#[gpui::test]
fn the_emoji_glyph_paints_no_background(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--kind", "emoji"]));
    let record = find(&records, "repo-avatar");
    assert_eq!(record.background, Paint::None);
    let text = record.text.expect("the emoji is a text run");
    assert_eq!(text.content, "\u{1f98a}");
}

/// **`empty` blanks the letter's text run while the box keeps its size and
/// background** — the one real state flag this surface has, reached through
/// the real `label` prop at its edge value.
///
/// The one-line mutation: in the surface's `Params::repo_avatar`, drop the
/// `cell.has(StateFlag::Empty)` branch so `--flags empty` fell through to
/// `label_of(cell.content)` — this test's `assert_eq!(text.content, "")`
/// would then read `"RE"` (the default `--content normal`) and fail.
#[gpui::test]
fn empty_blanks_the_letter_but_keeps_the_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));

    let root = at(&records, "repo-avatar");
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let record = find(&records, "repo-avatar");
    assert!(matches!(record.background, Paint::Solid(_)), "the box still paints");
    let text = record.text.expect("boxed_text always records a run, even an empty one");
    assert_eq!(text.content, "");
}

/// `--content` moves the letter's own run, and `empty` overrides it.
#[gpui::test]
fn content_moves_the_letter_and_empty_overrides_it(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut widths = Vec::new();
    for content in ["short", "normal", "overflow"] {
        let records = measure(cx, cell(&["--content", content]));
        let record = find(&records, "repo-avatar");
        widths.push(record.text.expect("a run").width);
    }
    assert!(widths[0] < widths[1] && widths[1] < widths[2], "{widths:?}");

    let overridden = measure(cx, cell(&["--content", "overflow", "--flags", "empty"]));
    assert_eq!(find(&overridden, "repo-avatar").text.unwrap().content, "");
}
