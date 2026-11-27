// PLATFORM: ios CoreText cannot read woff2 and a Swift package's resources are not carried into
// PLATFORM: ios needs TrueType, so the web's fonts ship in that form instead of woff2.
static FONTS: [&[u8]; 2] = [
    include_bytes!("../assets/CalSansUI.ttf"),
    include_bytes!("../assets/InstrumentSerif.ttf"),
];

pub(crate) fn font(index: usize) -> Option<&'static [u8]> {
    FONTS.get(index).copied()
}

#[cfg(test)]
mod tests {
    use super::*;

    const TRUETYPE_MAGICS: &[&[u8]] = &[&[0, 1, 0, 0], b"true", b"OTTO"];

    #[test]
    fn both_fonts_are_truetype_files() {
        for index in 0..2 {
            let bytes = font(index).expect("both fonts are embedded");
            assert!(
                TRUETYPE_MAGICS.iter().any(|magic| bytes.starts_with(magic)),
                "font {index} starts like a TrueType file"
            );
        }
    }

    #[test]
    fn index_past_the_last_font_is_none() {
        assert!(font(2).is_none(), "only two fonts are embedded");
    }
}
