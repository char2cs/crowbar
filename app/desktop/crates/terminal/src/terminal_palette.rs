//! The colours a terminal draws with.

use crate::rgb::Rgb;
use crate::xterm_color::xterm_color;

const ANSI_COLORS: usize = 16;

/// Default text, default background, cursor and the 16 ANSI colours; the 240 others are fixed.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TerminalPalette {
    /// Colour of text with no explicit colour.
    pub foreground: Rgb,
    /// Colour behind everything; cells with this background are not filled.
    pub background: Rgb,
    /// Cursor colour.
    pub cursor: Rgb,
    /// Black, red, green, yellow, blue, magenta, cyan, white, then their bright forms.
    pub ansi: [Rgb; ANSI_COLORS],
}

impl TerminalPalette {
    /// The colour of palette entry `index` (0 to 255).
    pub fn indexed(
        &self,
        index: u8,
    ) -> Rgb {
        self.ansi
            .get(usize::from(index))
            .copied()
            .unwrap_or_else(|| xterm_color(index))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn palette() -> TerminalPalette {
        TerminalPalette {
            foreground: Rgb::from_hex(0xf2_f0ea),
            background: Rgb::from_hex(0x1c_1c1a),
            cursor: Rgb::from_hex(0xf2_f0ea),
            ansi: std::array::from_fn(|index| Rgb::from_hex(index as u32)),
        }
    }

    #[test]
    fn low_indexes_come_from_the_theme_and_high_from_the_cube() {
        let palette = palette();
        assert_eq!(palette.indexed(9), Rgb::from_hex(9));
        assert_eq!(palette.indexed(196), Rgb::from_hex(0xff_0000));
    }
}
