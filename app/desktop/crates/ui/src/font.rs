//! Embedded font files and the family names they register.

use std::borrow::Cow;

use gpui::App;

use crate::error::Error;

/// Family of the UI text (`CalSansUI`, variable weight 300 to 700).
pub const UI_FAMILY: &str = "Cal Sans UI";
/// Family of headings (`font-heading`).
pub const HEADING_FAMILY: &str = "Instrument Serif";
/// Family of code, terminal and keyboard hints (`font-mono`).
pub const MONO_FAMILY: &str = "Geist Mono";

const FILES: [&[u8]; 3] = [
    include_bytes!("../assets/fonts/CalSansUI.ttf"),
    include_bytes!("../assets/fonts/InstrumentSerif.ttf"),
    include_bytes!("../assets/fonts/GeistMono-Variable.ttf"),
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
}
