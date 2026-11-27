pub(crate) mod assists;
pub(crate) mod highlighting;
pub(crate) mod save_format;
pub(crate) mod word_wrap;

use editor::WhitespaceMode;
use gpui::{App, Global, Pixels, px};

use crate::{Assists, SaveFormat, WordWrap};

const DEFAULT_FONT_SIZE: f32 = 14.0;
const DEFAULT_LINE_HEIGHT: f32 = 1.4;
const DEFAULT_TAB_SIZE: usize = 2;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Settings {
    pub font_size: f32,
    pub line_height: f32,
    pub tab_size: usize,
    pub word_wrap: WordWrap,
    pub line_numbers: bool,
    pub indent_guides: bool,
    pub whitespace: WhitespaceMode,
    pub assists: Assists,
    pub save_format: SaveFormat,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            font_size: DEFAULT_FONT_SIZE,
            line_height: DEFAULT_LINE_HEIGHT,
            tab_size: DEFAULT_TAB_SIZE,
            word_wrap: WordWrap::Off,
            line_numbers: true,
            indent_guides: true,
            whitespace: WhitespaceMode::None,
            assists: Assists::default(),
            save_format: SaveFormat::AsTyped,
        }
    }
}

impl Global for Settings {}

impl Settings {
    pub fn install(
        self,
        cx: &mut App,
    ) {
        if cx.try_global::<Self>() == Some(&self) {
            return;
        }
        cx.set_global(self);
    }

    #[must_use]
    pub fn of(cx: &App) -> Self {
        cx.try_global::<Self>().copied().unwrap_or_default()
    }

    #[must_use]
    pub fn row_height(&self) -> Pixels {
        px((self.font_size * self.line_height).ceil())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_row_is_the_font_size_times_the_line_height_rounded_up() {
        let settings = Settings::default();
        assert_eq!(settings.row_height(), px(20.0), "14 x 1.4 = 19.6");
        let larger = Settings {
            font_size: 18.0,
            line_height: 1.5,
            ..Settings::default()
        };
        assert_eq!(larger.row_height(), px(27.0));
    }
}
