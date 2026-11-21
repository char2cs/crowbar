//! The text and fill colours one grid cell draws with.

use alacritty_terminal::term::cell::{Cell, Flags};

use crate::snapshot::palette::ColorSource;
use crate::snapshot::palette::Rgb;

const XTERM_CLEAR_AS_INK: Rgb = Rgb::from_hex(0);

pub(crate) fn cell_colors(
    colors: &ColorSource<'_>,
    cell: &Cell,
) -> (Rgb, Rgb) {
    let flags = cell.flags;
    let mut foreground = colors.resolve(cell.fg, flags);
    let mut background = colors.resolve(cell.bg, Flags::empty());
    if flags.contains(Flags::INVERSE) {
        std::mem::swap(&mut foreground, &mut background);
        foreground = inverse_ink(colors, foreground);
    }
    if flags.contains(Flags::DIM) {
        foreground = foreground.dimmed();
    }
    if flags.contains(Flags::HIDDEN) {
        foreground = background;
    }
    (foreground, background)
}

fn inverse_ink(
    colors: &ColorSource<'_>,
    ink: Rgb,
) -> Rgb {
    if ink == colors.palette.background {
        return XTERM_CLEAR_AS_INK;
    }
    ink
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::snapshot::Palette;

    #[test]
    fn inverse_text_on_the_default_background_is_black_as_xterm_draws_it() {
        let palette = Palette {
            foreground: Rgb::from_hex(0xf2_f0ea),
            background: Rgb::from_hex(0x1c_1c1a),
            cursor: Rgb::from_hex(3),
            ansi: [Rgb::from_hex(4); 16],
        };
        let colors = ColorSource {
            palette: &palette,
            overrides: &alacritty_terminal::term::color::Colors::default(),
        };
        let cell = Cell {
            flags: Flags::INVERSE,
            ..Cell::default()
        };
        assert_eq!(
            cell_colors(&colors, &cell),
            (XTERM_CLEAR_AS_INK, palette.foreground)
        );
    }
}
