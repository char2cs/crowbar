//! The terminal palette of the installed theme.

use gpui::{App, Hsla, Rgba};
use terminal::{Rgb, TerminalPalette};
use ui::{Surface, Theme};

use crate::ansi_colors::ansi_colors;

const BYTE_RANGE: f32 = 255.0;

pub(crate) fn terminal_palette(cx: &App) -> TerminalPalette {
    let theme = Theme::of(cx);
    let tokens = theme.palette(Surface::Window);
    TerminalPalette {
        foreground: rgb_of(tokens.foreground()),
        background: rgb_of(tokens.pane_background()),
        cursor: rgb_of(tokens.foreground()),
        ansi: ansi_colors(theme.appearance()),
    }
}

pub(crate) fn is_dark(cx: &App) -> bool {
    Theme::of(cx).appearance() == ui::Appearance::Dark
}

pub(crate) fn hex_text(color: Rgb) -> String {
    format!("#{:06x}", color.to_hex())
}

pub(crate) fn hsla_of(color: Rgb) -> Hsla {
    Hsla::from(Rgba {
        r: f32::from(color.red) / BYTE_RANGE,
        g: f32::from(color.green) / BYTE_RANGE,
        b: f32::from(color.blue) / BYTE_RANGE,
        a: 1.0,
    })
}

fn rgb_of(color: Hsla) -> Rgb {
    let rgba = Rgba::from(color);
    let channel = |value: f32| (value.clamp(0.0, 1.0) * BYTE_RANGE).round() as u8;
    Rgb {
        red: channel(rgba.r),
        green: channel(rgba.g),
        blue: channel(rgba.b),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn colours_round_trip_through_gpui() {
        let colour = Rgb::from_hex(0x80_a36f);
        assert_eq!(rgb_of(hsla_of(colour)), colour);
        assert_eq!(hex_text(colour), "#80a36f");
    }
}
