use std::borrow::Cow;

use gpui::App;

use crate::error::Error;

pub const UI_FAMILY: &str = "Cal Sans UI";
pub const HEADING_FAMILY: &str = "Instrument Serif";
pub const MONO_FAMILY: &str = "Geist Mono";

#[must_use]
pub fn registered_family(setting: &str) -> &str {
    match setting {
        "CalSansUI" => UI_FAMILY,
        "Geist Mono Variable" => MONO_FAMILY,
        other => other,
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

pub fn register(cx: &App) -> Result<(), Error> {
    let fonts = FILES.iter().map(|bytes| Cow::Borrowed(*bytes)).collect();
    cx.text_system().add_fonts(fonts).map_err(Error::Fonts)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    #[cfg(target_os = "linux")]
    use gpui::{FontStyle, FontWeight};

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
        weight: FontWeight,
        style: FontStyle,
    ) -> (FontWeight, FontStyle) {
        use gpui::PlatformTextSystem as _;

        let system = gpui_wgpu::CosmicTextSystem::new_without_system_fonts(UI_FAMILY);
        let files = FILES.iter().map(|bytes| Cow::Borrowed(*bytes)).collect();
        system.add_fonts(files).expect("embedded fonts");
        let font = gpui::Font {
            weight,
            style,
            ..gpui::font(family.to_owned())
        };
        let face = system.font_id(&font).expect("a face of the family");
        system.font_weight_and_style(face).expect("the face")
    }

    #[cfg(target_os = "linux")]
    const FACE_CASES: [(&str, FontWeight, FontStyle); 7] = [
        (UI_FAMILY, FontWeight::NORMAL, FontStyle::Normal),
        (UI_FAMILY, FontWeight::MEDIUM, FontStyle::Normal),
        (UI_FAMILY, FontWeight::SEMIBOLD, FontStyle::Normal),
        (UI_FAMILY, FontWeight::BOLD, FontStyle::Normal),
        (UI_FAMILY, FontWeight::MEDIUM, FontStyle::Italic),
        (MONO_FAMILY, FontWeight::NORMAL, FontStyle::Normal),
        (MONO_FAMILY, FontWeight::BOLD, FontStyle::Normal),
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
}
