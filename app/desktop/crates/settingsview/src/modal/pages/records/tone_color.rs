//! The palette colour of a [`Tone`].

use gpui::Hsla;
use ui::Palette;

use settings::Tone;

pub(crate) fn tone_color(
    tone: Tone,
    palette: &Palette,
) -> Hsla {
    match tone {
        Tone::Neutral => palette.muted_foreground(),
        Tone::Success => palette.success(),
        Tone::Warning => palette.warning(),
        Tone::Error => palette.destructive(),
        Tone::Pending => palette.info(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ui::Surface;
    use ui::Theme;

    #[test]
    fn each_tone_maps_to_its_palette_token() {
        let palette = Theme::default().palette(Surface::Window);
        assert_eq!(
            tone_color(Tone::Neutral, palette),
            palette.muted_foreground()
        );
        assert_eq!(tone_color(Tone::Success, palette), palette.success());
        assert_eq!(tone_color(Tone::Warning, palette), palette.warning());
        assert_eq!(tone_color(Tone::Error, palette), palette.destructive());
        assert_eq!(tone_color(Tone::Pending, palette), palette.info());
    }
}
