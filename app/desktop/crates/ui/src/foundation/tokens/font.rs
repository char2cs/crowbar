//! Embedded font files and the family names they register.

use std::borrow::Cow;

use gpui::{App, Font, FontFallbacks};

use crate::error::Error;

/// Family of the UI text (`CalSansUI`, variable weight 300 to 700).
pub const UI_FAMILY: &str = "Cal Sans UI";
/// Family of headings (`font-heading`).
pub const HEADING_FAMILY: &str = "Instrument Serif";
/// Family of code, terminal and keyboard hints (`font-mono`).
pub const MONO_FAMILY: &str = "Geist Mono";

/// Colour emoji families, in the order a browser on each platform prefers them. Text that may hold
/// emoji falls back to these before the system's own fallback, which on Linux offers `DejaVu Sans`'s
/// monochrome emoji first.
pub const EMOJI_FAMILIES: [&str; 2] = ["Noto Color Emoji", "Apple Color Emoji"];

/// The UI font with the colour emoji families as its fallbacks, for text that is mostly emoji,
/// such as a space's emoji mark.
#[must_use]
pub fn emoji_font() -> Font {
    let fallbacks = EMOJI_FAMILIES
        .iter()
        .map(|family| (*family).to_owned())
        .collect();
    Font {
        fallbacks: Some(FontFallbacks::from_fonts(fallbacks)),
        ..gpui::font(UI_FAMILY)
    }
}

const FILES: [&[u8]; 3] = [
    include_bytes!("../../../assets/fonts/CalSansUI.ttf"),
    include_bytes!("../../../assets/fonts/InstrumentSerif.ttf"),
    include_bytes!("../../../assets/fonts/GeistMono-Variable.ttf"),
];

/// Registers the embedded families with the text system of `cx`.
///
/// # Errors
///
/// Returns [`Error::Fonts`] when the text system cannot parse a font file.
pub fn register(cx: &App) -> Result<(), Error> {
    let fonts = FILES.iter().map(|bytes| Cow::Borrowed(*bytes)).collect();
    cx.text_system().add_fonts(fonts).map_err(Error::Fonts)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn embedded_files_are_accepted_by_the_text_system(cx: &mut TestAppContext) {
        cx.update(|cx| assert!(register(cx).is_ok()));
    }

    #[test]
    fn family_names_match_the_font_name_tables() {
        for (family, bytes) in [
            (UI_FAMILY, FILES[0]),
            (HEADING_FAMILY, FILES[1]),
            (MONO_FAMILY, FILES[2]),
        ] {
            let needle: Vec<u8> = family.encode_utf16().flat_map(u16::to_be_bytes).collect();
            assert!(
                bytes
                    .windows(needle.len())
                    .any(|window| window == needle.as_slice()),
                "{family} not in its font file"
            );
        }
    }

    #[cfg(target_os = "linux")]
    fn font_of_q_falling_back_to(fallback: &str) -> gpui::FontId {
        use gpui::PlatformTextSystem as _;

        let system = gpui_wgpu::CosmicTextSystem::new_without_system_fonts(MONO_FAMILY);
        let fixtures: [&'static [u8]; 3] = [
            include_bytes!("../../../assets/fonts/fixtures/geist-mono-no-q.ttf"),
            include_bytes!("../../../assets/fonts/fixtures/cal-sans-ui-only-q.ttf"),
            include_bytes!("../../../assets/fonts/fixtures/instrument-serif-only-q.ttf"),
        ];
        let fonts = fixtures.iter().map(|bytes| Cow::Borrowed(*bytes)).collect();
        system.add_fonts(fonts).expect("fixture fonts");
        let mut font = gpui::font(MONO_FAMILY);
        font.fallbacks = Some(FontFallbacks::from_fonts(vec![fallback.to_owned()]));
        let primary = system.font_id(&font).expect("the primary face");
        let run = gpui::FontRun {
            len: 1,
            font_id: primary,
            letter_spacing: gpui::px(0.0),
        };
        let line = system.layout_line("Q", gpui::px(16.0), &[run]);
        line.runs.first().map(|run| run.font_id).expect("one run")
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn a_fallback_family_without_latin_letters_draws_what_it_covers() {
        assert_ne!(
            font_of_q_falling_back_to(HEADING_FAMILY),
            font_of_q_falling_back_to(UI_FAMILY),
            "each chain draws Q from its own fallback, as an emoji font draws its emoji"
        );
    }
}
