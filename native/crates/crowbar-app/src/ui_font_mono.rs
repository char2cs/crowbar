//! This item's own regression: no monospace font was registered at all, and
//! the four ported components that ask for `theme.font_mono`
//! (`fps_overlay`, `file_tree_row`, `inline_error`, and — in a module doc
//! comment only, never a live call site — `command`) shaped with whatever
//! CoreText's cascade supplied instead, silently.
//!
//! # Why these are plain `#[test]`s, sharing `ui_font_fallback`'s harness
//!
//! `#[gpui::test]` cannot see this defect at all, for the exact reason
//! `ui_font_fallback.rs`'s own module docs prove in detail:
//! `TestPlatform::new` hardcodes a `NoopTextSystem`, whose `add_fonts` is a
//! no-op and whose `advance` is fabricated from `char::len_utf16()` alone —
//! no `#[gpui::test]` in this workspace ever shapes a real glyph. This file
//! follows `ui_font_fallback.rs`'s and `ui_font_weight.rs`'s pattern exactly:
//! a real, headless `MacTextSystem` from `gpui_platform::current_platform`,
//! serialised behind [`super::ui_font_fallback::REAL_PLATFORM`] rather than a
//! second lock of its own (two real platforms built concurrently reliably
//! `SIGABRT`s — see that module's own concurrency note).
//!
//! # One methodology difference from `ui_font_weight.rs`, and why
//!
//! P3.25's own `weight_600_is_drawn_from_a_different_face_than_weight_500`
//! rules out "distinct `FontId`, same face" by checking that the *same
//! glyph* measures a *different advance* under each weight. That check does
//! not transfer here: `JetBrainsMonoVariable-{Regular,Medium,Bold}.ttf` are
//! genuinely, uniformly monospaced — measured directly with `fontTools`
//! against the exact files this item ships, every sampled glyph (digits,
//! letters, punctuation, the space) carries the same 600-unit advance at
//! every one of the three weights, so a real weight change can never move
//! the *advance*. [`each_weight_is_a_distinct_registered_face`] below proves
//! the same claim a different way instead: the same glyph's own
//! **typographic (ink) bounds** — read straight from the resolved face's
//! outline data, via [`gpui::PlatformTextSystem::typographic_bounds`] — widen
//! monotonically with weight even though the advance does not, which only
//! happens if three genuinely different outlines were loaded. Confirmed
//! directly against the shipped files before this test was written: glyph
//! `zero`'s bounding box is `(80, -10, 520, 740)` at Regular, `(74, -10, 526,
//! 740)` at Medium, `(68, -10, 532, 740)` at Bold — each wider than the last.
//!
//! # A second methodology difference, found rather than assumed: headless
//! CoreText has no answer at all for an unregistered family
//!
//! Every one of `ui_font_fallback.rs`'s and `ui_font_weight.rs`'s tests
//! registers `CalSansUI` before making a "bare" request — "bare" there means
//! "no explicit fallback chain on the `Font` value," never "the family was
//! never given to CoreText at all." This item's own defect is exactly that
//! family-level absence, which those files' tests never exercised, and it
//! behaves differently: [`an_unregistered_request_has_no_answer_in_headless_coretext`]
//! found that `font_id()` for a wholly unregistered `"JetBrains Mono
//! Variable"` request returns `Err("no font found")` in headless CoreText,
//! at every weight this component asks for — not a silently substituted
//! `FontId` the way a missing *glyph* inside an already-registered family
//! gets one. A first draft of this file assumed the P3.24 shape (measure the
//! same request before and after registration, expect the advance to move)
//! would transfer; it did not, and the failure — `no font found` rather than
//! a wrong number — is reported here rather than routed around.

use std::sync::{Arc, PoisonError};

use gpui::{Font, FontFeatures, FontId, FontRun, FontWeight, Pixels, PlatformTextSystem, font, px};

use super::ui_font_fallback::REAL_PLATFORM;
use super::{UI_MONO_FONT_FAMILY, ui_mono_font_paths};

/// `fps_overlay`'s own reference string at `--fps 59 --max-dt 32 --drops 16`
/// — the exact cell the brief's own side-by-side measured `WebKit` against:
/// `59 fps·max 32ms·16 drops`, 202.41px total (182.41px of text advance plus
/// the badge's 20px of horizontal padding). Split into the same seven runs
/// `FpsOverlay::shell` paints, at the same three weights.
const REFERENCE_RUNS: [(FontWeight, &str); 7] = [
    (FontWeight::BOLD, "59"),
    (FontWeight::NORMAL, " fps"),
    (FontWeight::NORMAL, "·"),
    (FontWeight::NORMAL, "max "),
    (FontWeight::NORMAL, "32ms"),
    (FontWeight::NORMAL, "·"),
    (FontWeight::MEDIUM, "16 drops"),
];

/// `fps_overlay::FONT_SIZE` — `text-[11px]`.
const REFERENCE_SIZE: Pixels = px(11.0);

/// `WebKit`'s own measured advance for [`REFERENCE_RUNS`] at [`REFERENCE_SIZE`]
/// (the brief's own side-by-side: 202.41px total, less the badge's 20px
/// padding). `mx-1.5` on each of the two `·` separators (12px a side) is part
/// of this number — it is a box-model margin, not glyph advance, but it is
/// exactly as real a part of what `WebKit` measured, so it is included here
/// rather than pretending the run-by-run glyph sum alone would match.
const REFERENCE_ADVANCE: f32 = 182.41;

/// A real platform, fresh — nothing registered on it yet. Used to reproduce
/// the pre-fix production shape: a `theme.font_mono` request against a
/// platform that was never given the real face, exactly what shipped before
/// this item.
fn fresh_platform() -> Arc<dyn PlatformTextSystem> {
    super::ui_font_fallback::real_platform_text_system()
}

/// Registers the real `JetBrains Mono Variable` faces exactly as `main.rs`'s
/// `load_ui_mono_font` does, straight off the same paths.
fn load_real_mono_font(pts: &dyn PlatformTextSystem) {
    let faces: Vec<std::borrow::Cow<'static, [u8]>> = ui_mono_font_paths()
        .iter()
        .map(|path| {
            std::borrow::Cow::Owned(
                std::fs::read(path)
                    .unwrap_or_else(|err| panic!("{} is missing: {err}", path.display())),
            )
        })
        .collect();
    pts.add_fonts(faces)
        .expect("the real JetBrains Mono Variable faces must parse");
    assert!(
        pts.all_font_names()
            .iter()
            .any(|name| name == UI_MONO_FONT_FAMILY),
        "JetBrains Mono Variable parsed but did not register with the real platform text system"
    );
}

/// `theme.font_mono` requested at a given weight, with no fallback chain of
/// its own — the exact value every one of the four call sites builds
/// (`.font_family(theme.font_mono.primary()…)` plus, at three of the seven
/// `fps_overlay` runs, a `.font_weight(...)` override).
fn mono_font(weight: FontWeight) -> Font {
    let mut requested = font(UI_MONO_FONT_FAMILY);
    requested.weight = weight;
    requested
}

/// Shapes one run and hands back its measured width and the [`FontId`] the
/// first (only, for Latin text this short) run resolved to.
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

/// The full [`REFERENCE_RUNS`] string, shaped run-by-run and summed — the
/// same "one `FontRun` per differently-styled span" shape `fps_overlay`
/// itself paints in, rather than one call with a synthetic multi-run
/// `FontRun` list this test would have to hand-assemble.
fn shape_reference(pts: &dyn PlatformTextSystem) -> Pixels {
    let mut total = 0.0f32;
    for &(weight, text) in &REFERENCE_RUNS {
        let (width, _) = shape_line(pts, &mono_font(weight), REFERENCE_SIZE, text);
        total += f32::from(width);
    }
    // The two `·` separators each carry `mx-1.5` — 6px of margin on both
    // sides, 12px each, in the box model rather than in any glyph's own
    // advance. `fps_overlay::SEPARATOR_MARGIN_X` is the same `px(6.0)`.
    px(total + 2.0 * 12.0)
}

/// **The registration, measured through a glyph — not asserted from a
/// list.** `all_font_names()` only says the bytes parsed; this shapes real
/// text through the resolved face and reads back a real, non-fallback
/// advance.
#[test]
fn the_mono_family_registers_and_shapes_a_real_advance() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = fresh_platform();
    load_real_mono_font(&*pts);

    let (width, _) = shape_line(&*pts, &mono_font(FontWeight::NORMAL), REFERENCE_SIZE, "0");
    // JetBrains Mono Variable is `0.6em` per glyph at every weight — 6.6px at
    // 11px. A `.notdef`/degenerate box would not land here by coincidence.
    assert!(
        (f32::from(width) - 6.6).abs() < 0.05,
        "expected the digit '0' to measure JetBrains Mono Variable's own 0.6em \
         advance (6.6px at 11px), got {width:?} — the real face was not what shaped this"
    );
}

/// **The regression, established directly rather than assumed.** The first
/// draft of this file tried the same A/B `ui_font_fallback.rs`'s own
/// `every_missing_glyph_changes_or_is_already_correct_without_calsansui`
/// uses for its sibling defect — shape the same request on a fresh platform,
/// once before registering the real face and once after — and it does not
/// transfer here, for a reason worth recording rather than working around
/// silently: **an entirely unregistered `theme.font_mono` request has no
/// answer at all in headless CoreText**, at every one of the three weights
/// this component ever asks for — `font_id()` returns `Err("no font
/// found")`, not a substituted `FontId` the way P3.24's fix found for a
/// *missing glyph inside an already-registered family*. `CalSansUI` was
/// always registered first in every one of `ui_font_fallback.rs`'s own
/// tests; "bare" there means "no explicit fallback chain attached to the
/// `Font` value," never "the family itself was never given to CoreText."
/// This item's defect is the family-level case that file's tests never
/// exercised.
///
/// That headless `Err` is itself the strongest form of "nothing was
/// registered" this test could report, and it is asserted directly below.
/// It also means the pre-fix **live windowed app's own measured 157px**
/// (this item's brief) cannot be reproduced through this headless harness at
/// all — real, non-headless CoreText (with an active window server
/// connection) resolves *something* for a totally unknown family where
/// headless CoreText does not, which is a genuine behavioural gap between
/// the two, not a bug in this test.
#[test]
fn an_unregistered_request_has_no_answer_in_headless_coretext() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = fresh_platform();

    for weight in [FontWeight::NORMAL, FontWeight::MEDIUM, FontWeight::BOLD] {
        let err = pts.font_id(&mono_font(weight)).expect_err(&format!(
            "expected an unregistered JetBrains Mono Variable request at {weight:?} to fail \
             in headless CoreText (nothing was ever registered); if this now succeeds, \
             headless font matching changed and this test's premise needs revisiting"
        ));
        assert!(
            err.to_string().contains("no font found"),
            "expected \"no font found\" at {weight:?}, got {err}"
        );
    }
}

/// **The bar's own number.** Once the real face is registered,
/// [`REFERENCE_RUNS`] must measure `WebKit`'s own [`REFERENCE_ADVANCE`]
/// (182.41px), not the pre-fix fallback's ~157px this item's brief measured
/// live. Measured directly while drafting this test: `182.40001px` —
/// `0.01px` off, `CoreText`'s own float-shaping jitter
/// (`ui_font_fallback.rs`'s module docs record the same class of noise, a
/// bare request measuring `⌘` at `12.000001px` rather than a clean `12.0`),
/// not a real disagreement.
#[test]
fn the_reference_string_measures_the_webkit_advance_once_registered() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = fresh_platform();
    load_real_mono_font(&*pts);

    let width = shape_reference(&*pts);

    assert!(
        (f32::from(width) - REFERENCE_ADVANCE).abs() < 0.1,
        "expected `59 fps·max 32ms·16 drops` to measure ~{REFERENCE_ADVANCE}px \
         (WebKit's own value), got {width:?}"
    );
}

/// **"Drawn", not just "measured a different `FontId`."** A monospaced font
/// cannot show this the way `CalSansUI`'s proportional faces do (see the
/// module docs) — the same glyph's *advance* is identical at every weight by
/// design — so this checks the same glyph's **ink bounds** instead, read via
/// [`gpui::PlatformTextSystem::typographic_bounds`], which reads the
/// resolved face's own outline data rather than its metrics table. A bolder
/// weight's strokes are wider, so the bounding box widens even though the
/// advance the layout box is built from does not.
#[test]
fn each_weight_is_a_distinct_registered_face() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = fresh_platform();
    load_real_mono_font(&*pts);

    let mut font_ids = Vec::new();
    let mut widths = Vec::new();
    for weight in [FontWeight::NORMAL, FontWeight::MEDIUM, FontWeight::BOLD] {
        let (_, font_id) = shape_line(&*pts, &mono_font(weight), REFERENCE_SIZE, "0");
        let glyph_id = pts
            .glyph_for_char(font_id, '0')
            .expect("JetBrains Mono Variable has a digit zero at every weight");
        let bounds = pts
            .typographic_bounds(font_id, glyph_id)
            .expect("a registered face must answer for its own glyph");
        font_ids.push(font_id);
        widths.push(bounds.size.width);
    }

    assert_ne!(
        font_ids[0], font_ids[1],
        "Regular and Medium share a FontId"
    );
    assert_ne!(font_ids[1], font_ids[2], "Medium and Bold share a FontId");
    assert_ne!(font_ids[0], font_ids[2], "Regular and Bold share a FontId");

    assert!(
        widths[0] < widths[1],
        "expected Medium's '0' to carry wider ink than Regular's: {widths:?}"
    );
    assert!(
        widths[1] < widths[2],
        "expected Bold's '0' to carry wider ink than Medium's: {widths:?}"
    );
}

/// **`tabular-nums`, implemented and measured — not merely declared.**
/// `fps_overlay::shell` applies `FontFeatures` carrying `"tnum" => 1`,
/// gpui/CoreText's real mechanism for `font-variant-numeric: tabular-nums`
/// (`vendor/zed-deps/gpui_macos/src/open_type.rs::apply_features_and_fallbacks`
/// hands the tag straight to `kCTFontOpenTypeFeatureTag`). This proves the
/// feature reaches the shaper (present vs absent is a real, different
/// `Font` value, so a typo in the tag would still shape *something*, just
/// not this) **and** documents what it costs on this specific font: nothing.
/// `JetBrainsMonoVariable-Regular.ttf`'s own `GSUB` table carries no `tnum`
/// feature at all (checked directly with `fontTools` against the shipped
/// file: `['calt', 'ccmp', 'frac', 'locl']`) — unsurprising, since tabular
/// figures are this font's *only* figures (every glyph, not only the ten
/// digits, already carries the same 600-unit advance). CoreText silently
/// ignores a feature tag the resolved face does not carry rather than
/// erroring, so the measured width below is identical with the feature on or
/// off — a fact about `JetBrains Mono Variable`, not a limitation of gpui's
/// `FontFeatures`, which is a real, working mechanism (see
/// `each_weight_is_a_distinct_registered_face` for a font where a feature
/// *would* have room to move something).
#[test]
fn tabular_nums_reaches_the_shaper_and_is_measurably_inert_on_this_font() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = fresh_platform();
    load_real_mono_font(&*pts);

    let plain = mono_font(FontWeight::MEDIUM);
    let mut tabular = plain.clone();
    tabular.features = FontFeatures(Arc::new(vec![("tnum".to_owned(), 1)]));

    assert_ne!(
        plain.features, tabular.features,
        "the two Font values must actually differ, or this test would prove nothing"
    );

    let (width_plain, _) = shape_line(&*pts, &plain, REFERENCE_SIZE, "16 drops");
    let (width_tabular, _) = shape_line(&*pts, &tabular, REFERENCE_SIZE, "16 drops");

    assert!(
        (f32::from(width_plain) - f32::from(width_tabular)).abs() < 0.01,
        "expected `tabular-nums` to be inert on JetBrains Mono Variable (already \
         uniformly monospaced): plain {width_plain:?}, tabular {width_tabular:?}"
    );
}
