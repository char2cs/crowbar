//! User settings of the terminal.

mod ansi_colors;
mod channel_byte;
mod font;
mod theme_palette;

pub(crate) use font::{base_font, styled_font};
pub(crate) use theme_palette::{hex_text, hsla_of, is_dark, terminal_config, terminal_palette};

use terminal::CursorShape;
use ui::font::MONO_FAMILY;

const DEFAULT_FONT_SIZE: f32 = 14.0;
const MIN_FONT_SIZE: f32 = 8.0;
const MAX_FONT_SIZE: f32 = 32.0;
const DEFAULT_SCROLLBACK: usize = 10_000;
const DEFAULT_CURSOR_WIDTH: f32 = 2.0;

/// Font, spacing, scrollback and cursor of a terminal view.
#[derive(Debug, Clone, PartialEq)]
pub struct Settings {
    /// Font family; Geist Mono unless the user chose another.
    pub font_family: String,
    /// Font size in pixels, 8 to 32.
    pub font_size: f32,
    /// Line height as a multiple of the font's natural height.
    pub line_height: f32,
    /// Extra pixels after every glyph.
    pub letter_spacing: f32,
    /// Lines of scrollback.
    pub scrollback: usize,
    /// Cursor shape.
    pub cursor_shape: CursorShape,
    /// Whether the cursor blinks.
    pub cursor_blink: bool,
    /// Width of the bar cursor and thickness of the underline cursor, in pixels.
    pub cursor_width: f32,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            font_family: MONO_FAMILY.to_owned(),
            font_size: DEFAULT_FONT_SIZE,
            line_height: 1.0,
            letter_spacing: 0.0,
            scrollback: DEFAULT_SCROLLBACK,
            cursor_shape: CursorShape::Block,
            cursor_blink: true,
            cursor_width: DEFAULT_CURSOR_WIDTH,
        }
    }
}

impl Settings {
    /// The same settings with the font size moved by `delta` and kept within 8 to 32.
    #[must_use]
    pub fn zoomed(
        &self,
        delta: f32,
    ) -> Self {
        Self {
            font_size: (self.font_size + delta).clamp(MIN_FONT_SIZE, MAX_FONT_SIZE),
            ..self.clone()
        }
    }

    /// The same settings at the default font size.
    #[must_use]
    pub fn zoom_reset(&self) -> Self {
        Self {
            font_size: DEFAULT_FONT_SIZE,
            ..self.clone()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn zoom_is_clamped_and_resettable() {
        let settings = Settings::default();
        assert!((settings.zoomed(2.0).font_size - 16.0).abs() < f32::EPSILON);
        assert!((settings.zoomed(-100.0).font_size - 8.0).abs() < f32::EPSILON);
        assert!((settings.zoomed(100.0).font_size - 32.0).abs() < f32::EPSILON);
        assert!((settings.zoomed(6.0).zoom_reset().font_size - 14.0).abs() < f32::EPSILON);
    }
}
