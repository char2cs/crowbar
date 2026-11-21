//! The cell xterm's DOM renderer sizes from a font, as develop draws it.

use crate::settings::Settings;

// PLATFORM: linux WebKitGTK hands xterm whole hinted advances, ascents and descents.
pub(crate) fn cell_size(
    (advance, ascent, descent): (f32, f32, f32),
    settings: &Settings,
) -> (f32, f32) {
    let width = advance.round() + settings.letter_spacing.round();
    let height = ascent.round() + descent.round();
    (width, (height * settings.line_height).floor())
}

#[cfg(test)]
mod tests {
    use super::*;

    const UNITS_PER_EM: f32 = 1000.0;
    const GEIST_MONO_ADVANCE: f32 = 600.0;
    const GEIST_MONO_ASCENT: f32 = 1005.0;
    const GEIST_MONO_DESCENT: f32 = 295.0;

    fn geist_mono_cell(
        font_size: f32,
        line_height: f32,
    ) -> (f32, f32) {
        let scale = font_size / UNITS_PER_EM;
        let font = (
            GEIST_MONO_ADVANCE * scale,
            GEIST_MONO_ASCENT * scale,
            GEIST_MONO_DESCENT * scale,
        );
        let settings = Settings {
            font_size,
            line_height,
            ..Settings::default()
        };
        cell_size(font, &settings)
    }

    #[test]
    fn the_cell_is_the_one_develops_xterm_measures_for_geist_mono() {
        let cases = [
            (14.0, 1.0, (8.0, 18.0)),
            (16.0, 1.0, (10.0, 21.0)),
            (18.0, 1.0, (11.0, 23.0)),
            (14.0, 1.2, (8.0, 21.0)),
        ];
        for (font_size, line_height, cell) in cases {
            let measured = geist_mono_cell(font_size, line_height);
            assert_eq!(measured, cell, "{font_size}px at line height {line_height}");
        }
    }
}
