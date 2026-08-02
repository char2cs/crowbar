//! The fallback chain [`Theme::font_sans`] renders with — P3.24.
//!
//! # The defect this file fixes
//!
//! `main.rs` registers `CalSansUI-Regular.ttf`/`CalSansUI-Medium.ttf` under the
//! single family `CalSansUI`, and every `font_sans` call site handed gpui a
//! bare `gpui::font("CalSansUI")` — no [`FontFallbacks`]. `CalSansUI`'s own cmap
//! has no entry for the macOS modifier symbols (`⌘` U+2318, `⌥` U+2325, `⇧`
//! U+21E7, `⌃` U+2303, `⏎` U+23CE, `⌫` U+232B, `␛` U+241B), so gpui drew
//! `.notdef` — a box exactly 1em square — wherever the React app draws one of
//! them.
//!
//! `WebKit` never hits this: the CSS resolves through a fallback *stack*, not a
//! single family, and the OS supplies the glyph from a later entry. gpui has
//! the same mechanism ([`Font::fallbacks`]) — it was simply never populated.
//! [`ui_sans_font`] is now the one place that does, and every `font_sans` call
//! site in `components/` uses it instead of naming the family directly.
//!
//! # The stack, read off the real value rather than assumed
//!
//! `ui-font`'s CSS is `font-family: var(--app-font-family, var(--font-sans))`
//! (`web/src/index.css`). `--font-sans` alone
//! (`web/src/styles/theme.css`) is only `var(--app-font-family, 'CalSansUI',
//! sans-serif)` — but at runtime `--app-font-family` is *always* set, by
//! `applyBootstrapAppearance` in
//! `web/src/features/settings/lib/appearance-bootstrap.ts`, to
//! `buildFontVariable(cache.uiFontFamily, sansFallback)`. `uiFontFamily`
//! defaults to `DEFAULT_UI_FONT_FAMILY` = `"CalSansUI"`
//! (`web/src/features/settings/config/typography-defaults.ts`), and on
//! anything that is not Windows (this app is macOS-only), `sansFallback` is
//! `DEFAULT_SANS_FALLBACK`:
//!
//! ```text
//! ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI",
//! Roboto, "Helvetica Neue", Arial, sans-serif
//! ```
//!
//! So the stack the reference actually measures through is
//! `"CalSansUI", ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont,
//! "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif` — not the
//! two-entry `["CalSansUI", "sans-serif"]` [`Theme::font_sans`] itself carries
//! ([`FontFamily`] is transcribed straight from `theme.css`'s static
//! declaration, which is the *unconfigured* value CSS falls back to and not
//! the one the running app applies).
//!
//! # Translating that stack for CoreText
//!
//! [`FontFallbacks`] resolves each entry as a real CoreText family name
//! (`kCTFontFamilyNameAttribute`) — see
//! `native/vendor/zed-deps/gpui_macos/src/open_type.rs`'s
//! `generate_fallback_array`. CoreText has no concept of the CSS Fonts Level 4
//! generic keywords: `ui-sans-serif`, `system-ui`, `-apple-system` and
//! `BlinkMacSystemFont` are four *`WebKit`* spellings for the exact same thing on
//! macOS — the system UI font (San Francisco) — so passing any of them
//! literally would ask CoreText to match a family that does not exist.
//! `.AppleSystemUIFont` is the concrete name for that font: it is what
//! `NSFont.systemFont(ofSize:).fontName` returns, and this vendor already
//! trusts it — `gpui::font_name_with_fallbacks` maps gpui's own
//! `.SystemUIFont` to exactly this string for the same reason. The four
//! keywords collapse to that one real entry; the remaining concrete names
//! (`Segoe UI`, `Roboto`, `Helvetica Neue`, `Arial`) carry over literally,
//! even though on a real Mac they are never reached — `.AppleSystemUIFont`
//! already supplies every glyph `WebKit`'s system font does. The trailing CSS
//! generic `sans-serif` needs no explicit entry: `apply_features_and_fallbacks`
//! always appends CoreText's own computed default cascade
//! (`CTFontCopyDefaultCascadeListForLanguages`) after this list, which is the
//! same backstop `sans-serif` names.
use gpui::{Font, FontFallbacks, font};

use super::Theme;

/// [`UI_SANS_FALLBACKS`] translated into [`FontFallbacks`] once and reused —
/// see the module docs for how each entry was chosen.
const UI_SANS_FALLBACKS: [&str; 5] = [
    ".AppleSystemUIFont",
    "Segoe UI",
    "Roboto",
    "Helvetica Neue",
    "Arial",
];

/// The `gpui::Font` every `font_sans` call site paints text with.
///
/// Carries [`Theme::font_sans`]'s declared family (so `font.family` in a
/// snapshot still reports `"CalSansUI"`, unchanged) plus the fallback chain
/// above, so a glyph missing from `CalSansUI` resolves through CoreText's
/// cascade instead of drawing `.notdef`.
#[must_use]
pub fn ui_sans_font(theme: &Theme) -> Font {
    let mut resolved = font(theme.font_sans.primary().unwrap_or("sans-serif"));
    resolved.fallbacks = Some(FontFallbacks::from_fonts(
        UI_SANS_FALLBACKS
            .iter()
            .map(|name| (*name).to_owned())
            .collect(),
    ));
    resolved
}

#[cfg(test)]
mod tests {
    use super::{Theme, ui_sans_font};

    /// The declared family must stay exactly what `ANCHORS.md` v1.2 requires a
    /// snapshot to report — adding fallbacks must not change it.
    #[test]
    fn keeps_the_declared_family() {
        let resolved = ui_sans_font(&Theme::DARK);
        assert_eq!(resolved.family.as_ref(), "CalSansUI");
    }

    /// The one thing a unit test *can* pin without a real text system: the
    /// literal list, matched entry for entry against the module docs' reading
    /// of `appearance-bootstrap.ts`. Whether it actually resolves a missing
    /// glyph is a behavioural question a real font system has to answer — see
    /// `crowbar-app`'s `ui_font_fallback` tests, which measure it.
    #[test]
    fn carries_the_translated_webkit_stack() {
        let resolved = ui_sans_font(&Theme::LIGHT);
        let fallbacks = resolved.fallbacks.expect("fallbacks must be set");
        assert_eq!(
            fallbacks.fallback_list(),
            &[
                ".AppleSystemUIFont",
                "Segoe UI",
                "Roboto",
                "Helvetica Neue",
                "Arial"
            ],
        );
    }
}
