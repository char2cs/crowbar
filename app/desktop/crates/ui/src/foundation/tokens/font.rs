//! Embedded font files and the family names they register.
//!
//! GPUI draws a variable face at its default instance only, so each weight and the italic the UI
//! sets is a static face of its own, made by `tools/scripts/font-instances.py`.

use std::borrow::Cow;

use gpui::{App, Font, FontFallbacks};

use crate::error::Error;

/// Family of the UI text (`CalSansUI`): one static face per weight the tokens use, and a medium oblique.
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

const FILES: [&[u8]; 8] = [
    include_bytes!("../../../assets/fonts/CalSansUI-Regular.ttf"),
    include_bytes!("../../../assets/fonts/InstrumentSerif.ttf"),
    include_bytes!("../../../assets/fonts/GeistMono-Regular.ttf"),
    include_bytes!("../../../assets/fonts/CalSansUI-Medium.ttf"),
    include_bytes!("../../../assets/fonts/CalSansUI-SemiBold.ttf"),
    include_bytes!("../../../assets/fonts/CalSansUI-Bold.ttf"),
    include_bytes!("../../../assets/fonts/CalSansUI-MediumOblique.ttf"),
    include_bytes!("../../../assets/fonts/GeistMono-Bold.ttf"),
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

    const FAMILY_CASES: [(&str, usize); 3] =
        [(UI_FAMILY, 0), (HEADING_FAMILY, 1), (MONO_FAMILY, 2)];

    #[test]
    fn family_names_match_the_font_name_tables() {
        for (family, file) in FAMILY_CASES {
            let bytes = FILES[file];
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
    fn drawn_face(
        family: &str,
        weight: gpui::FontWeight,
        style: gpui::FontStyle,
    ) -> (gpui::FontWeight, gpui::FontStyle) {
        use gpui::PlatformTextSystem as _;

        let system = gpui_wgpu::CosmicTextSystem::new_without_system_fonts(UI_FAMILY);
        let files = FILES.iter().map(|bytes| Cow::Borrowed(*bytes)).collect();
        system.add_fonts(files).expect("embedded fonts");
        let font = Font {
            weight,
            style,
            ..gpui::font(family.to_owned())
        };
        let face = system.font_id(&font).expect("a face of the family");
        system.font_weight_and_style(face).expect("the face")
    }

    #[cfg(target_os = "linux")]
    const FACE_CASES: [(&str, gpui::FontWeight, gpui::FontStyle); 7] = [
        (UI_FAMILY, gpui::FontWeight::NORMAL, gpui::FontStyle::Normal),
        (UI_FAMILY, gpui::FontWeight::MEDIUM, gpui::FontStyle::Normal),
        (
            UI_FAMILY,
            gpui::FontWeight::SEMIBOLD,
            gpui::FontStyle::Normal,
        ),
        (UI_FAMILY, gpui::FontWeight::BOLD, gpui::FontStyle::Normal),
        (UI_FAMILY, gpui::FontWeight::MEDIUM, gpui::FontStyle::Italic),
        (
            MONO_FAMILY,
            gpui::FontWeight::NORMAL,
            gpui::FontStyle::Normal,
        ),
        (MONO_FAMILY, gpui::FontWeight::BOLD, gpui::FontStyle::Normal),
    ];

    #[cfg(target_os = "linux")]
    #[test]
    fn each_weight_and_the_italic_draw_from_a_face_of_their_own() {
        for (family, weight, style) in FACE_CASES {
            assert_eq!(
                drawn_face(family, weight, style),
                (weight, style),
                "{family} at {weight:?} {style:?} draws from its own static face"
            );
        }
    }

    #[cfg(target_os = "linux")]
    const SNOWMAN: char = '\u{2603}';

    #[cfg(target_os = "linux")]
    type Families = &'static [&'static str];

    #[cfg(target_os = "linux")]
    const DRAWN_BEFORE: [(&str, Families); 3] = [
        ("a fallback family without Latin letters", &[]),
        (
            "after a mono label drew the family without fallbacks",
            &[MONO_FAMILY],
        ),
        (
            "after the fallback family was asked for as a primary font",
            &[HEADING_FAMILY, UI_FAMILY],
        ),
    ];

    #[cfg(target_os = "linux")]
    fn fixture_system() -> gpui_wgpu::CosmicTextSystem {
        use gpui::PlatformTextSystem as _;

        let system = gpui_wgpu::CosmicTextSystem::new_without_system_fonts(MONO_FAMILY);
        let fixtures: [&'static [u8]; 3] = [
            include_bytes!("../../../assets/fonts/fixtures/geist-mono-no-q.ttf"),
            include_bytes!("../../../assets/fonts/fixtures/cal-sans-ui-only-snowman.ttf"),
            include_bytes!("../../../assets/fonts/fixtures/instrument-serif-only-snowman.ttf"),
        ];
        let fonts = fixtures.iter().map(|bytes| Cow::Borrowed(*bytes)).collect();
        system.add_fonts(fonts).expect("fixture fonts");
        system
    }

    #[cfg(target_os = "linux")]
    fn fixture_system_after(earlier: Families) -> gpui_wgpu::CosmicTextSystem {
        use gpui::PlatformTextSystem as _;

        let system = fixture_system();
        for family in earlier {
            let _resolved = system.font_id(&gpui::font(*family));
        }
        system
    }

    #[cfg(target_os = "linux")]
    fn ascent_of_snowman_face(
        system: &gpui_wgpu::CosmicTextSystem,
        fallbacks: &[&str],
    ) -> f32 {
        use gpui::PlatformTextSystem as _;

        let mut font = gpui::font(MONO_FAMILY);
        let chain: Vec<String> = fallbacks
            .iter()
            .map(|family| (*family).to_owned())
            .collect();
        font.fallbacks = (!chain.is_empty()).then(|| FontFallbacks::from_fonts(chain));
        let primary = system.font_id(&font).expect("the primary face");
        let run = gpui::FontRun {
            len: SNOWMAN.len_utf8(),
            font_id: primary,
        };
        let line = system.layout_line(&SNOWMAN.to_string(), gpui::px(16.0), &[run]);
        let face = line.runs.first().map(|run| run.font_id).expect("one run");
        system.font_metrics(face).ascent
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn each_fallback_chain_draws_from_its_own_families() {
        for (case, earlier) in DRAWN_BEFORE {
            let system = fixture_system_after(earlier);
            assert_ne!(
                ascent_of_snowman_face(&system, &[HEADING_FAMILY]),
                ascent_of_snowman_face(&system, &[UI_FAMILY]),
                "{case}: each chain draws the snowman from its own fallback, as the terminal's draws its emoji"
            );
        }
    }
}
