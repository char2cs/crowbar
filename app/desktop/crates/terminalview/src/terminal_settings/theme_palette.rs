//! The terminal palette of the installed theme.

use gpui::{App, Hsla, Rgba};
use terminal::Rgb;
use ui::{Surface, Theme};

use crate::terminal_settings::TerminalSettings;
use crate::terminal_settings::ansi_colors::ansi_colors;
use crate::terminal_settings::channel_byte::channel_byte;

const BYTE_RANGE: f32 = 255.0;

pub(crate) fn terminal_palette(cx: &App) -> terminal::Palette {
    let theme = Theme::of(cx);
    let tokens = theme.palette(Surface::Window);
    terminal::Palette {
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
    Rgb {
        red: channel_byte(rgba.r),
        green: channel_byte(rgba.g),
        blue: channel_byte(rgba.b),
    }
}

const INITIAL_COLUMNS: usize = 80;
const INITIAL_LINES: usize = 24;

pub(crate) fn terminal_config(
    settings: &TerminalSettings,
    cx: &App,
) -> terminal::Config {
    terminal::Config {
        columns: INITIAL_COLUMNS,
        lines: INITIAL_LINES,
        scrollback: settings.scrollback,
        cursor_shape: settings.cursor_shape,
        cursor_blink: settings.cursor_blink,
        palette: terminal_palette(cx),
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
