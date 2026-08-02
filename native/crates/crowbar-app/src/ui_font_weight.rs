//! P3.25's regression: the native app could not render any weight above 500.
//!
//! # The defect
//!
//! `main.rs` used to register exactly two static faces —
//! `CalSansUI-Regular.ttf` (`wght: 400`) and `CalSansUI-Medium.ttf`
//! (`wght: 500`) — while the React app loads `web/public/fonts/CalSansUI.woff2`,
//! a **variable** font declared over `wght: 400–700`. `font-kit`'s
//! `find_best_match` (`vendor/zed-deps/gpui_macos/src/text_system.rs`) can only
//! choose among the faces it was actually given, so a `font-weight: 600`
//! request (`font-semibold`, 41 call sites in `web/src`, `--font-weight-semibold:
//! 600`) had nothing closer than the 500 face and silently rendered as Medium.
//! `WebKit` never hits this: its `@font-face` loads the variable font directly
//! and shapes a real semibold.
//!
//! The fix adds a third static instance, `CalSansUI-SemiBold.ttf`
//! (`wght: 600`), by the same `fontTools` recipe `UI_FONT_FILES`'s own doc
//! comment records for the first two — see that comment for the exact
//! provenance and the OS/2 details.
//!
//! # Why these are plain `#[test]`s, sharing `ui_font_fallback`'s harness
//!
//! `#[gpui::test]` cannot see this defect at all: `TestPlatform` hardcodes
//! `NoopTextSystem`, whose `advance` is `600.0 * glyph_id.0` for *every*
//! character regardless of weight — no `#[gpui::test]` in this workspace ever
//! shapes a real glyph, and `ui_font_fallback.rs`'s module docs already prove
//! that in detail for the sibling defect. This file follows that file's
//! pattern exactly: a real, headless `MacTextSystem` from
//! `gpui_platform::current_platform(true)`, serialised behind
//! [`super::ui_font_fallback::REAL_PLATFORM`] rather than a second lock of its
//! own — that module's docs record that constructing two real platforms
//! *concurrently* reliably `SIGABRT`s, and a second `Mutex<()>` here would not
//! serialise against the first one.
use std::sync::PoisonError;

use crowbar_ui::Theme;
use gpui::{Font, FontId, FontRun, FontWeight, Pixels, PlatformTextSystem, px};

use super::UI_FONT_FAMILY;
use super::ui_font_fallback::{REAL_PLATFORM, load_real_calsansui, real_platform_text_system};

/// `search_toggle_icons.rs`'s `WEIGHT` — the anchor already merged into
/// `rewrite/rust`, `Aa` at `ui-text-xs`/11px. Its native capture matched the
/// 500 column (`14.19`) rather than `WebKit`'s 600 (`14.48`) before this fix.
const AA_SIZE: Pixels = px(11.0);
const AA_TEXT: &str = "Aa";
const AA_EXPECTED: f32 = 14.48;

/// The failing field from the held `dialog` verdict this item was diagnosed
/// from (`dialog-title.text_width`, `native/QUEUE.md`) — not capturable on
/// this branch (`--surface dialog` only exists on
/// `native/p3.21-dialog-sheet-rebased`), so measured directly here instead.
const ADD_REPOSITORY_SIZE: Pixels = px(20.0);
const ADD_REPOSITORY_TEXT: &str = "Add repository";
const ADD_REPOSITORY_EXPECTED: f32 = 149.4;

/// How close a real shaped measurement has to land to the pinned `WebKit`
/// value to count as a match. Tighter than the oracle differ's own `±1.0` on
/// `text_width` (this is a unit test pinning one real number, not a tolerance
/// budget for cross-engine hinting noise), and wide enough for CoreText's own
/// float shaping jitter — `ui_font_fallback.rs` found a bare request off a
/// clean integer by `0.000001px`.
const WEBKIT_TOLERANCE: f32 = 0.1;

/// [`crowbar_ui::ui_sans_font`] carries no weight of its own — every call site
/// sets `.weight` after building it (see `avatar.rs`, `card.rs`,
/// `search_toggle_icons.rs`) — so this is what every one of those calls
/// actually shapes with once a weight is attached.
fn weighted_font(theme: &Theme, weight: FontWeight) -> Font {
    let mut font = crowbar_ui::ui_sans_font(theme);
    font.weight = weight;
    font
}

/// Shapes a whole line (not [`super::ui_font_fallback`]'s single-character
/// `shape`, which asserts exactly one glyph) and hands back its measured
/// width and the [`FontId`] the *first* run resolved to. `"Add repository"`
/// and `"Aa"` are both plain Latin text with no reason to split into more
/// than one run, but nothing here assumes that beyond reading run `0`.
fn shape_line(
    pts: &dyn PlatformTextSystem,
    requested: &Font,
    size: Pixels,
    text: &str,
) -> (Pixels, FontId) {
    let font_id = pts
        .font_id(requested)
        .unwrap_or_else(|err| panic!("resolving {requested:?}: {err}"));
    let layout = pts.layout_line(
        text,
        size,
        &[FontRun {
            len: text.len(),
            font_id,
        }],
    );
    assert!(
        !layout.runs.is_empty(),
        "expected at least one run shaping {text:?}, got none"
    );
    (layout.width, layout.runs[0].font_id)
}

/// **The bar's first number.** `"Add repository"` at 20px weight 600 must
/// measure `WebKit`'s `149.4`, not the pre-fix Medium-face value of `143.82`.
#[test]
fn add_repository_at_600_measures_the_webkit_advance() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = real_platform_text_system();
    load_real_calsansui(&*pts);

    let semibold = weighted_font(&Theme::DARK, FontWeight::SEMIBOLD);
    let (width, _) = shape_line(&*pts, &semibold, ADD_REPOSITORY_SIZE, ADD_REPOSITORY_TEXT);

    assert!(
        (f32::from(width) - ADD_REPOSITORY_EXPECTED).abs() < WEBKIT_TOLERANCE,
        "expected {ADD_REPOSITORY_TEXT:?} at weight 600 to measure ~{ADD_REPOSITORY_EXPECTED}px \
         (WebKit's own value), got {width:?}"
    );
}

/// **The bar's second number.** `"Aa"` at 11px weight 600 must measure
/// `WebKit`'s `14.48`, not the pre-fix Medium-face value of `14.19` —
/// `search-toggle-icons`' own anchor, merged and recorded as converged while
/// carrying this defect (`native/QUEUE.md`).
#[test]
fn aa_at_600_measures_the_webkit_advance() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = real_platform_text_system();
    load_real_calsansui(&*pts);

    let semibold = weighted_font(&Theme::DARK, FontWeight::SEMIBOLD);
    let (width, _) = shape_line(&*pts, &semibold, AA_SIZE, AA_TEXT);

    assert!(
        (f32::from(width) - AA_EXPECTED).abs() < WEBKIT_TOLERANCE,
        "expected {AA_TEXT:?} at weight 600 to measure ~{AA_EXPECTED}px (WebKit's own \
         value), got {width:?}"
    );
}

/// **The metric match is necessary but not sufficient** — this is the "drawn,
/// not just measured" check the brief asks for. A weight-600 request has to
/// resolve to a *different* [`FontId`] than weight 500, and — because a
/// different `FontId` alone is weak evidence (`ui_font_fallback.rs`'s own
/// `a_present_glyph_still_shapes_through_calsansui_itself` records that
/// attaching a fallback chain alone mints a new `FontId` for a glyph the
/// cascade never touches) — the *same* glyph (`'A'`) has to measure a
/// genuinely different advance under the two ids, which only happens if two
/// different outlines were actually selected and shaped.
#[test]
fn weight_600_is_drawn_from_a_different_face_than_weight_500() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = real_platform_text_system();
    load_real_calsansui(&*pts);

    let medium = weighted_font(&Theme::DARK, FontWeight::MEDIUM);
    let semibold = weighted_font(&Theme::DARK, FontWeight::SEMIBOLD);

    let (width_medium, font_id_medium, _) = super::ui_font_fallback::shape(&*pts, &medium, 'A');
    let (width_semibold, font_id_semibold, _) =
        super::ui_font_fallback::shape(&*pts, &semibold, 'A');

    assert_ne!(
        font_id_medium, font_id_semibold,
        "weight 500 and weight 600 requests for the same family resolved to the same \
         FontId — the 600 request cannot be reaching a distinct face"
    );
    assert!(
        (f32::from(width_medium) - f32::from(width_semibold)).abs() > 0.05,
        "'A' measured {width_medium:?} at weight 500 and {width_semibold:?} at weight \
         600 — too close to be two different outlines; a distinct FontId alone does not \
         prove a distinct *drawn* weight (see this test's own doc comment)"
    );
}

/// The declared family stays `CalSansUI` at every weight — `ANCHORS.md`
/// requires the *declared* family, and `find_best_match` choosing a different
/// face inside the family must not change what a snapshot reports.
#[test]
fn the_semibold_face_still_declares_the_themes_family() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = real_platform_text_system();
    load_real_calsansui(&*pts);

    let semibold = weighted_font(&Theme::DARK, FontWeight::SEMIBOLD);
    assert_eq!(semibold.family.as_ref(), UI_FONT_FAMILY);
    // Resolves at all — `font_id` errors rather than silently falling back,
    // so this is itself a check that the family+weight pair is servable.
    pts.font_id(&semibold)
        .expect("CalSansUI at weight 600 must resolve now that the SemiBold face exists");
}
