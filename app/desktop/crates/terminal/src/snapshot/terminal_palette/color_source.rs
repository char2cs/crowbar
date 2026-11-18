//! Turns the emulator's colour references into concrete colours.

use alacritty_terminal::term::cell::Flags;
use alacritty_terminal::term::color::Colors;
use alacritty_terminal::vte::ansi::{Color, NamedColor, Rgb as AnsiRgb};

use crate::snapshot::terminal_palette::TerminalPalette;
use crate::snapshot::terminal_palette::rgb::Rgb;

const NORMAL_COLORS: usize = 8;
const DIM_FIRST: usize = NamedColor::DimBlack as usize;
const DIM_LAST: usize = NamedColor::DimWhite as usize;
const BACKGROUND: usize = NamedColor::Background as usize;
const CURSOR: usize = NamedColor::Cursor as usize;
const DIM_FOREGROUND: usize = NamedColor::DimForeground as usize;

#[derive(Clone, Copy)]
pub(crate) struct ColorSource<'a> {
    pub(crate) palette: &'a TerminalPalette,
    pub(crate) overrides: &'a Colors,
}

impl ColorSource<'_> {
    pub(crate) fn resolve(
        &self,
        color: Color,
        flags: Flags,
    ) -> Rgb {
        match color {
            Color::Spec(spec) => from_ansi(spec),
            Color::Indexed(index) => self.entry(usize::from(index)),
            Color::Named(named) => self.named(named as usize, flags),
        }
    }

    fn named(
        &self,
        index: usize,
        flags: Flags,
    ) -> Rgb {
        let index = brightened(index, flags);
        if (DIM_FIRST..=DIM_LAST).contains(&index) {
            return self.entry(index - DIM_FIRST).dimmed();
        }
        self.entry(index)
    }

    fn entry(
        &self,
        index: usize,
    ) -> Rgb {
        match self.overrides[index] {
            Some(redefined) => from_ansi(redefined),
            None => self.default_entry(index),
        }
    }

    fn default_entry(
        &self,
        index: usize,
    ) -> Rgb {
        let palette = self.palette;
        if let Some(ansi) = palette.ansi.get(index) {
            return *ansi;
        }
        match index {
            BACKGROUND => palette.background,
            CURSOR => palette.cursor,
            DIM_FOREGROUND => palette.foreground.dimmed(),
            _ => u8::try_from(index).map_or(palette.foreground, |byte| palette.indexed(byte)),
        }
    }
}

fn brightened(
    index: usize,
    flags: Flags,
) -> usize {
    if flags.contains(Flags::BOLD) && index < NORMAL_COLORS {
        return index + NORMAL_COLORS;
    }
    index
}

fn from_ansi(color: AnsiRgb) -> Rgb {
    Rgb {
        red: color.r,
        green: color.g,
        blue: color.b,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn palette() -> TerminalPalette {
        let ansi = std::array::from_fn(|index| {
            Rgb::from_hex(0x10_0000 + u32::try_from(index).unwrap_or_default())
        });
        TerminalPalette {
            foreground: Rgb::from_hex(0xaa_aaaa),
            background: Rgb::from_hex(0x11_1111),
            cursor: Rgb::from_hex(0xcc_cccc),
            ansi,
        }
    }

    fn cases(palette: &TerminalPalette) -> Vec<(Color, Flags, Rgb)> {
        let none = Flags::empty();
        let named = |color| Color::Named(color);
        vec![
            (named(NamedColor::Red), none, palette.ansi[1]),
            (named(NamedColor::Red), Flags::BOLD, palette.ansi[9]),
            (named(NamedColor::Foreground), none, palette.foreground),
            (named(NamedColor::Background), none, palette.background),
            (named(NamedColor::DimRed), none, palette.ansi[1].dimmed()),
            (Color::Indexed(196), none, Rgb::from_hex(0xff_0000)),
            (
                Color::Spec(AnsiRgb { r: 1, g: 2, b: 3 }),
                none,
                Rgb::from_hex(0x01_0203),
            ),
        ]
    }

    #[test]
    fn colours_resolve_from_the_palette() {
        let palette = palette();
        let overrides = Colors::default();
        let source = ColorSource {
            palette: &palette,
            overrides: &overrides,
        };
        for (color, flags, expected) in cases(&palette) {
            assert_eq!(
                source.resolve(color, flags),
                expected,
                "{color:?} {flags:?}"
            );
        }
    }

    #[test]
    fn program_overrides_win() {
        let palette = palette();
        let mut overrides = Colors::default();
        overrides[NamedColor::Foreground as usize] = Some(AnsiRgb { r: 9, g: 9, b: 9 });
        let source = ColorSource {
            palette: &palette,
            overrides: &overrides,
        };
        let resolved = source.resolve(Color::Named(NamedColor::Foreground), Flags::empty());
        assert_eq!(resolved.to_hex(), 0x09_0909);
    }
}
