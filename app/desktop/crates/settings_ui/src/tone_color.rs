//! The palette colour of a [`Tone`].

use gpui::Hsla;
use ui::Palette;

use crate::tone::Tone;

pub(crate) fn tone_color(tone: Tone, palette: &Palette) -> Hsla {
    match tone {
        Tone::Neutral => palette.muted_foreground(),
        Tone::Success => palette.success(),
        Tone::Warning => palette.warning(),
        Tone::Error => palette.destructive(),
        Tone::Pending => palette.info(),
    }
}

#[cfg(test)]
#[path = "tone_color_tests.rs"]
mod tests;
