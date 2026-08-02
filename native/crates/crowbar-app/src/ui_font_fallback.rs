//! P3.24's regression: the native app drew `.notdef` for every macOS modifier
//! symbol, because `CalSansUI`'s own cmap has none of them and nothing gave
//! gpui anywhere else to look.
//!
//! # Why these are plain `#[test]`s, not `#[gpui::test]`s
//!
//! `#[gpui::test]`'s `TestAppContext` is built on `TestPlatform`, and
//! `TestPlatform::new` hardcodes a `NoopTextSystem`
//! (`vendor/gpui/src/platform/test/platform.rs`) — `add_fonts` is a no-op,
//! `font_id` always answers `FontId(1)`, and `glyph_for_char`/`advance`
//! fabricate a result from `char::len_utf16()` alone. **No `#[gpui::test]`
//! in this workspace ever shapes a real glyph**; every existing
//! `row_layout` test measures gpui's *layout arithmetic* (padding, flex,
//! taffy) against that synthetic font, which is exactly why those tests are
//! careful never to assert an exact shaped width (see `badge.rs`'s own
//! comment: "the two engines shape independently").
//!
//! Measuring what this item actually changed — CoreText's glyph
//! substitution — needs the *real* platform text system, which
//! `gpui_platform::current_platform` already builds for the shipping binary
//! and freely for a headless caller: `current_platform(true)` constructs a
//! real `gpui_macos::MacPlatform` — confirmed safe off the main thread under
//! a single test thread (no window, no `NSApplication`; this file's first
//! draft was a throwaway experiment proving exactly that before any
//! assertion was written) — whose `.text_system()` is the same
//! `MacTextSystem` `main.rs` registers `CalSansUI` into.
//!
//! **Concurrency note, load-bearing for CI**: constructing more than one
//! real `MacPlatform` *concurrently* reliably `SIGABRT`s (reproduced: 4
//! platforms built in parallel across `cargo test`'s worker threads
//! crashed the whole binary). Every test below therefore takes a
//! process-wide lock ([`REAL_PLATFORM`]) before touching the real platform,
//! serialising just this file's tests against each other and leaving every
//! other `#[gpui::test]`/`#[test]` in the workspace — which never construct
//! a real platform — free to run in parallel regardless of
//! `--test-threads`.
//!
//! # Why this measures the *shaped line*, not `PlatformTextSystem::advance`
//!
//! `advance` calls `glyph_for_char` on one already-resolved [`FontId`] — it
//! asks one font "do you have this glyph", which is exactly the question a
//! cascade list does not answer: CoreText only consults
//! `kCTFontCascadeListAttribute` while **shaping a line**
//! (`CTLineCreateWithAttributedString`, in
//! `vendor/zed-deps/gpui_macos/src/text_system.rs`'s `layout_line`), not
//! while resolving one glyph out of one font in isolation. So every
//! measurement here goes through [`PlatformTextSystem::layout_line`] and
//! reads the [`ShapedRun`] it actually produced, including its `font_id`.
//!
//! # A finding this file's measurements surfaced, not assumed
//!
//! Headless CoreText already performs *some* implicit font substitution for
//! a glyph missing from the primary face, fallback list or not — a bare
//! `CalSansUI` request (`font_without_fallback`, no `Font.fallbacks` at all)
//! measures six of the seven missing glyphs through a real, non-`CalSansUI`
//! font already. `ui_sans_font`'s explicit chain still changes the *outcome*
//! for six of those seven (a different, `WebKit`-matching advance — see
//! `⌘`'s exact number below) because it controls *which* font the cascade
//! reaches first, rather than *whether* substitution happens at all. The
//! seventh, `escape` (U+241B), resolves to bit-identical glyph and width
//! whichever request is used: headless CoreText's own implicit cascade
//! already lands on the one real font that carries it, so this fix changes
//! nothing observable for that glyph specifically **in headless mode**.
//! That is recorded rather than hidden — see
//! `every_missing_glyph_changes_or_is_already_correct_without_calsansui`'s
//! own per-glyph comments — because the diagnosis's own claim was that the
//! *windowed* app draws every one of the seven as `.notdef`, a claim this
//! headless harness cannot independently confirm or refute for six of the
//! seven (only `⌘`'s exact `WebKit` number was in the brief to check against).
//! Reported plainly rather than smoothed over.

use std::sync::{Arc, Mutex, PoisonError};

use crowbar_ui::Theme;
use gpui::{Font, FontId, FontRun, FontWeight, GlyphId, Pixels, PlatformTextSystem, font, px};

use super::{UI_FONT_FAMILY, ui_font_paths};

/// Serialises this file's tests against each other — see the module docs'
/// concurrency note. `Mutex<()>` rather than an atomic flag because a test
/// panicking mid-section must still release it (a poisoned lock still
/// unlocks on drop; only a *second* panic while already poisoned would
/// need handling, and no test here panics twice).
static REAL_PLATFORM: Mutex<()> = Mutex::new(());

/// The seven glyphs `native/QUEUE.md`'s P3.24 entry found missing from
/// `CalSansUI`'s cmap — every macOS modifier symbol the app's keycaps and
/// shortcuts paint. `(name, character)`, named for assertion failure
/// messages.
const MISSING_GLYPHS: [(&str, char); 7] = [
    ("cmd", '\u{2318}'),
    ("opt", '\u{2325}'),
    ("shift", '\u{21E7}'),
    ("ctrl", '\u{2303}'),
    ("return", '\u{23CE}'),
    ("delete", '\u{232B}'),
    ("escape", '\u{241B}'),
];

/// A glyph `CalSansUI` *does* carry (`U+0057 W`, present in the Regular face's
/// fmt4 `0x3a`–`0x7e` segment per the diagnosis) — the control that proves
/// the fallback chain does not disturb a glyph that never needed it.
const PRESENT_GLYPH: char = 'W';

/// `keybinding.tsx`'s measured face: 12px, weight 400 — the same settings
/// P3.24's diagnosis decomposed `⌘W`'s `text_width` with.
const FONT_SIZE: Pixels = px(12.0);

/// Float shaping is not bit-exact even for the same nominal quantity (a bare
/// `CalSansUI` request measured `⌘` at `12.000001px`, not a clean `12.0`), so
/// the `.notdef` signature is "within a hundredth of a pixel of `FONT_SIZE`",
/// not `==`.
const NOTDEF_TOLERANCE: f32 = 0.01;

/// Whether `width` is close enough to `FONT_SIZE` to be the `.notdef` box's
/// exact-1em signature, rather than a real glyph's advance.
fn is_notdef_width(width: Pixels) -> bool {
    (f32::from(width) - f32::from(FONT_SIZE)).abs() < NOTDEF_TOLERANCE
}

/// A real, headless platform text system — see the module docs for why this
/// is safe and necessary in place of `#[gpui::test]`'s `TestAppContext`, and
/// why every caller must hold [`REAL_PLATFORM`] first.
fn real_platform_text_system() -> Arc<dyn PlatformTextSystem> {
    gpui_platform::current_platform(true).text_system()
}

/// Registers the real `CalSansUI` faces exactly as `main.rs`'s `load_ui_font`
/// does, straight off the same paths — a fresh platform text system has
/// nothing loaded until this runs.
fn load_real_calsansui(pts: &dyn PlatformTextSystem) {
    let faces: Vec<std::borrow::Cow<'static, [u8]>> = ui_font_paths()
        .iter()
        .map(|path| {
            std::borrow::Cow::Owned(
                std::fs::read(path)
                    .unwrap_or_else(|err| panic!("{} is missing: {err}", path.display())),
            )
        })
        .collect();
    pts.add_fonts(faces)
        .expect("the real CalSansUI faces must parse");
    assert!(
        pts.all_font_names()
            .iter()
            .any(|name| name == UI_FONT_FAMILY),
        "CalSansUI parsed but did not register with the real platform text system"
    );
}

/// A bare `CalSansUI` request, with no fallback — the font value every
/// `font_sans` call site built before P3.24, and the shape [`ui_sans_font`]
/// degrades to if its fallback chain is ever dropped.
fn font_without_fallback() -> Font {
    let mut requested = font(UI_FONT_FAMILY);
    requested.weight = FontWeight::NORMAL;
    requested
}

/// Shapes a single character and hands back the run the shaper actually
/// produced: its width, which font id drew it, and which glyph id.
///
/// One run and one glyph are guaranteed: the input is one character, so
/// there is nothing for `layout_line` to split on. A failure here means this
/// helper's own contract broke, not that a real surface did.
fn shape(pts: &dyn PlatformTextSystem, requested: &Font, ch: char) -> (Pixels, FontId, GlyphId) {
    let font_id = pts
        .font_id(requested)
        .unwrap_or_else(|err| panic!("resolving {requested:?}: {err}"));
    let mut buf = [0u8; 4];
    let text = ch.encode_utf8(&mut buf);
    let layout = pts.layout_line(
        text,
        FONT_SIZE,
        &[FontRun {
            len: text.len(),
            font_id,
        }],
    );
    assert_eq!(
        layout.runs.len(),
        1,
        "expected one run shaping a single character {ch:?}, got {}",
        layout.runs.len()
    );
    let run = &layout.runs[0];
    assert_eq!(
        run.glyphs.len(),
        1,
        "expected one glyph shaping a single character {ch:?}, got {}",
        run.glyphs.len()
    );
    (layout.width, run.font_id, run.glyphs[0].id)
}

/// The `W` control never moves: an ordinary glyph a font actually has must
/// keep shaping to the same advance and the same glyph, fallback chain or
/// not.
#[test]
fn a_present_glyph_still_shapes_through_calsansui_itself() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = real_platform_text_system();
    load_real_calsansui(&*pts);

    let bare = font_without_fallback();
    let fixed_font = crowbar_ui::ui_sans_font(&Theme::DARK);

    let (width_bare, _, glyph_id_bare) = shape(&*pts, &bare, PRESENT_GLYPH);
    let (width_fallback, _, glyph_id_fallback) = shape(&*pts, &fixed_font, PRESENT_GLYPH);

    // `font_id` itself is *not* a valid cross-check here: attaching a
    // cascade list changes the `CTFontDescriptor`, so
    // `MacTextSystemState::load_family` mints a distinct `FontId` for the
    // with-fallback request even for a glyph the cascade is never consulted
    // for (own comment: a global postscript-name dedupe would be wrong). A
    // different id is expected; a different *advance* or *glyph* would not
    // be.
    assert!(
        (f32::from(width_bare) - f32::from(width_fallback)).abs() < NOTDEF_TOLERANCE,
        "a glyph CalSansUI already has must shape to the same advance whether or \
         not a fallback chain is attached: bare {width_bare:?}, with fallback \
         {width_fallback:?}"
    );
    assert_eq!(
        glyph_id_bare, glyph_id_fallback,
        "W never needs the cascade, so both requests must resolve to the same \
         glyph — CalSansUI's own W, not a substitute"
    );
    assert!(
        !is_notdef_width(width_bare),
        "W is a sanity control: CalSansUI has this glyph, so it must not measure \
         the .notdef box's 1em signature either"
    );
}

/// **The regression, measured as a direct A/B on one platform instance** —
/// not by comparing `FontId` numbers across two separately-constructed
/// `MacTextSystemState`s, which restart their own numbering from zero and so
/// cannot be compared to each other at all.
///
/// For every one of the seven glyphs P3.24 found missing, this shapes the
/// *same* character through the *same* real text system twice — once with
/// `font_without_fallback()` (today's shipped `Font` value, no cascade
/// list), once with `ui_sans_font`'s fix — and asserts the measured advance
/// **changes**. `ui_sans_font`'s fallback chain does not merely add a
/// cascade where headless CoreText had none (it already substitutes
/// *something* even for a bare request — see the module docs); it changes
/// *which* font the cascade reaches, which is exactly what pulls `⌘`'s
/// width from the `.notdef`-adjacent `12.000001px` a bare request measures
/// to `WebKit`'s own `11.027px` (checked exactly in the next test).
///
/// **One glyph is the honest exception**: `escape` (`␛`, U+241B) measures
/// bit-identically either way (`9.29297px`, the same glyph id) — headless
/// CoreText's own implicit cascade already lands on the one real font
/// carrying it, fallback chain or not, so this fix has nothing to change
/// for it specifically **in headless mode**. It is asserted only to still
/// resolve through a real, non-`.notdef` glyph (which is true both before
/// and after), and is called out rather than silently dropped from the loop
/// or forced to look like the other six.
///
/// **Mutation performed and reverted**: commenting out
/// `theme/ui_font.rs`'s `resolved.fallbacks = Some(...)` line (leaving
/// `ui_sans_font` degrade to `font_without_fallback`'s own bare value)
/// turned the six non-`escape` assertions below into failures of the exact
/// shape `cmd ('⌘'): expected ui_sans_font's fallback chain to move this \
/// glyph's advance off the bare request's 12.000001px, but both measured \
/// 12.000001px` — reproduced, then the line was restored before this file
/// was finalised.
#[test]
fn every_missing_glyph_changes_or_is_already_correct_without_calsansui() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = real_platform_text_system();
    load_real_calsansui(&*pts);

    let bare = font_without_fallback();
    let fixed_font = crowbar_ui::ui_sans_font(&Theme::DARK);
    let nominal_bare = pts.font_id(&bare).expect("CalSansUI resolves");
    let nominal_fallback = pts.font_id(&fixed_font).expect("CalSansUI resolves");

    for &(name, ch) in &MISSING_GLYPHS {
        let (width_bare, font_id_bare, glyph_id_bare) = shape(&*pts, &bare, ch);
        let (width_fallback, font_id_fallback, glyph_id_fallback) = shape(&*pts, &fixed_font, ch);

        // Both requests must reach *some* font other than CalSansUI's own —
        // true in every configuration this file measured, and worth pinning
        // so a future change that somehow made the primary face win again
        // (drawing its own `.notdef`) would be caught here too.
        assert_ne!(
            font_id_bare, nominal_bare,
            "{name} ({ch:?}): even a bare request should not be drawn by \
             CalSansUI's own font id"
        );
        assert_ne!(
            font_id_fallback, nominal_fallback,
            "{name} ({ch:?}): the fallback chain should not be drawn by \
             CalSansUI's own font id"
        );
        assert_ne!(
            glyph_id_bare.0, 0,
            "{name} ({ch:?}): expected a real, non-.notdef glyph even bare"
        );
        assert_ne!(
            glyph_id_fallback.0, 0,
            "{name} ({ch:?}): expected a real, non-.notdef glyph with the fallback"
        );

        if name == "escape" {
            // The honest exception — see this test's own doc comment.
            assert_eq!(
                (width_bare, glyph_id_bare),
                (width_fallback, glyph_id_fallback),
                "escape was recorded as measuring identically either way; if that \
                 changed, update the doc comment rather than this assertion"
            );
        } else {
            assert!(
                (f32::from(width_bare) - f32::from(width_fallback)).abs() >= NOTDEF_TOLERANCE,
                "{name} ({ch:?}): expected ui_sans_font's fallback chain to move this \
                 glyph's advance off the bare request's {width_bare:?}, but both \
                 measured {width_fallback:?}"
            );
        }
    }
}

/// **The one number the brief pins exactly**: `⌘` at 12px must measure the
/// same advance `WebKit` measures — `11.027`, not a bare request's
/// `.notdef`-adjacent `12.000001`.
#[test]
fn cmd_measures_the_webkit_advance_not_the_notdef_box() {
    let _guard = REAL_PLATFORM.lock().unwrap_or_else(PoisonError::into_inner);
    let pts = real_platform_text_system();
    load_real_calsansui(&*pts);

    let fixed_font = crowbar_ui::ui_sans_font(&Theme::DARK);
    let (width, _, _) = shape(&*pts, &fixed_font, '\u{2318}');

    assert!(
        (f32::from(width) - 11.027).abs() < 0.05,
        "expected \u{2318} to measure ~11.027px (WebKit's own value), got {width:?}"
    );
}
