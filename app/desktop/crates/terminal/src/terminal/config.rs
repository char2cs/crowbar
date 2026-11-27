use alacritty_terminal::term::Config as EmulatorConfig;
use alacritty_terminal::vte::ansi::{CursorShape as EmulatorShape, CursorStyle};

use crate::snapshot::CursorShape;
use crate::snapshot::Palette;

const WORD_SEPARATORS: &str = " ()[]{}',\"`│|<>\t";

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Config {
    pub columns: usize,
    pub lines: usize,
    pub scrollback: usize,
    pub cursor_shape: CursorShape,
    pub cursor_blink: bool,
    pub palette: Palette,
}

impl Config {
    pub(crate) fn emulator_config(&self) -> EmulatorConfig {
        let shape = match self.cursor_shape {
            CursorShape::Block => EmulatorShape::Block,
            CursorShape::Underline => EmulatorShape::Underline,
            CursorShape::Bar => EmulatorShape::Beam,
        };
        let style = CursorStyle {
            shape,
            blinking: self.cursor_blink,
        };
        EmulatorConfig {
            scrolling_history: self.scrollback,
            default_cursor_style: style,
            semantic_escape_chars: WORD_SEPARATORS.to_owned(),
            ..EmulatorConfig::default()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::snapshot::Rgb;

    #[test]
    fn emulator_config_carries_history_and_cursor() {
        let black = Rgb::from_hex(0);
        let palette = Palette {
            foreground: black,
            background: black,
            cursor: black,
            ansi: [black; 16],
        };
        let config = Config {
            columns: 80,
            lines: 24,
            scrollback: 5000,
            cursor_shape: CursorShape::Bar,
            cursor_blink: true,
            palette,
        };
        let emulator = config.emulator_config();
        assert_eq!(emulator.scrolling_history, 5000);
        assert_eq!(emulator.default_cursor_style.shape, EmulatorShape::Beam);
        assert!(emulator.default_cursor_style.blinking);
    }
}
