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
#[path = "font_tests.rs"]
mod tests;
