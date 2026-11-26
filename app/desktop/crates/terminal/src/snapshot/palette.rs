//! The colours a terminal draws with.

mod color_source;
mod rgb;

pub(crate) use color_source::{ColorSource, cell_colors};
pub use rgb::Rgb;

const ANSI_COLORS: usize = 16;
const CUBE_START: u8 = 16;
const GRAY_START: u8 = 232;
const CUBE_SIDE: u8 = 6;
const CUBE_BASE: u8 = 55;
const CUBE_STEP: u8 = 40;
const GRAY_BASE: u8 = 8;
const GRAY_STEP: u8 = 10;

/// Default text, default background, cursor and the 16 ANSI colours; the 240 others are fixed.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Palette {
    /// Colour of text with no explicit colour.
    pub foreground: Rgb,
    /// Colour behind everything; cells with this background are not filled.
    pub background: Rgb,
    /// Cursor colour.
    pub cursor: Rgb,
    /// Black, red, green, yellow, blue, magenta, cyan, white, then their bright forms.
    pub ansi: [Rgb; ANSI_COLORS],
}

impl Palette {
    pub(crate) fn indexed(
        &self,
        index: u8,
    ) -> Rgb {
        self.ansi
            .get(usize::from(index))
            .copied()
            .unwrap_or_else(|| xterm_color(index))
    }
}

fn xterm_color(index: u8) -> Rgb {
    if index >= GRAY_START {
        return gray(GRAY_BASE + GRAY_STEP * (index - GRAY_START));
    }
    let offset = index.saturating_sub(CUBE_START);
    Rgb {
        red: cube_level(offset / (CUBE_SIDE * CUBE_SIDE)),
        green: cube_level(offset / CUBE_SIDE % CUBE_SIDE),
        blue: cube_level(offset % CUBE_SIDE),
    }
}

fn gray(level: u8) -> Rgb {
    Rgb {
        red: level,
        green: level,
        blue: level,
    }
}

fn cube_level(step: u8) -> u8 {
    match step {
        0 => 0,
        _ => CUBE_BASE + CUBE_STEP * step,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn low_indexes_come_from_the_theme_and_high_from_the_xterm_cube_and_ramp() {
        let ansi =
            std::array::from_fn(|index| Rgb::from_hex(u32::try_from(index).unwrap_or_default()));
        let black = Rgb::from_hex(0);
        let palette = Palette {
            foreground: black,
            background: black,
            cursor: black,
            ansi,
        };
        let cases = [
            (9, 0x00_0009),
            (16, 0x00_0000),
            (21, 0x00_00ff),
            (196, 0xff_0000),
            (231, 0xff_ffff),
            (232, 0x08_0808),
            (255, 0xee_eeee),
        ];
        for (index, hex) in cases {
            assert_eq!(palette.indexed(index).to_hex(), hex, "index {index}");
        }
    }
}
