//! The user's editor settings, in develop's units and with develop's defaults.

use gpui::{App, Global, Pixels, px};
use crate::WordWrap;

mod whitespace_mode;

pub use whitespace_mode::WhitespaceMode;

const DEFAULT_FONT_SIZE: f32 = 14.0;
const DEFAULT_LINE_HEIGHT: f32 = 1.4;
const DEFAULT_TAB_SIZE: usize = 2;

/// Font size, spacing, indentation and what every editor view draws besides the text.
///
/// The shell installs it as a global; every open `EditorView` follows it live. The font family
/// is the theme's code font, which the same `fontFamily` setting chooses.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Settings {
    /// Font size in pixels.
    pub font_size: f32,
    /// Line height as a multiple of the font size, as develop's `editorLineHeight`.
    pub line_height: f32,
    /// Columns an indent takes; indents are typed as spaces.
    pub tab_size: usize,
    /// Whether long lines wrap at the pane's width.
    pub word_wrap: WordWrap,
    /// Whether the gutter shows line numbers.
    pub line_numbers: bool,
    /// Whether vertical guides mark the indentation levels.
    pub indent_guides: bool,
    /// Which spaces and tabs show a marker.
    pub whitespace: WhitespaceMode,
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
        }
    }
}

impl Global for Settings {}

impl Settings {
    /// Makes these the settings every editor view draws with, now and from then on; the same
    /// settings again leave the views alone.
    pub fn install(
        self,
        cx: &mut App,
    ) {
        if cx.try_global::<Self>() == Some(&self) {
            return;
        }
        cx.set_global(self);
    }

    /// The settings installed in `cx`, or develop's defaults before any are.
    #[must_use]
    pub fn of(cx: &App) -> Self {
        cx.try_global::<Self>().copied().unwrap_or_default()
    }

    /// The pixel height of a row: the font size times the line height, rounded up as develop's
    /// `calculateLineHeight` does.
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
