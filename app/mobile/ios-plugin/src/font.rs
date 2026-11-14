// PLATFORM: CoreText cannot read woff2 and a Swift package's resources are not carried into
// PLATFORM: the app, so the web's fonts ship as TrueType.
static FONTS: [&[u8]; 2] = [
    include_bytes!("../assets/CalSansUI.ttf"),
    include_bytes!("../assets/InstrumentSerif.ttf"),
];

pub(crate) fn font(index: usize) -> Option<&'static [u8]> {
    FONTS.get(index).copied()
}

#[cfg(test)]
#[path = "font_tests.rs"]
mod tests;
