mod color_source;
mod rgb;
mod xterm_color;

pub(crate) use color_source::ColorSource;
pub use rgb::Rgb;

use crate::snapshot::palette::xterm_color::xterm_color;

const ANSI_COLORS: usize = 16;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Palette {
    pub foreground: Rgb,
    pub background: Rgb,
    pub cursor: Rgb,
    pub ansi: [Rgb; ANSI_COLORS],
}

impl Palette {
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

    fn palette() -> Palette {
        let ansi =
            std::array::from_fn(|index| Rgb::from_hex(u32::try_from(index).unwrap_or_default()));
        Palette {
            foreground: Rgb::from_hex(0xf2_f0ea),
            background: Rgb::from_hex(0x1c_1c1a),
            cursor: Rgb::from_hex(0xf2_f0ea),
            ansi,
        }
    }

    #[test]
    fn low_indexes_come_from_the_theme_and_high_from_the_cube() {
        let palette = palette();
        assert_eq!(palette.indexed(9), Rgb::from_hex(9));
        assert_eq!(palette.indexed(196), Rgb::from_hex(0xff_0000));
    }
}
