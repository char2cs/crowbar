//! The fonts a terminal draws with.

use gpui::{Font, FontFallbacks, FontStyle, FontWeight, font};
use terminal::StyleFlags;

use crate::terminal_settings::TerminalSettings;

const FALLBACKS: [&str; 7] = [
    "Symbols Nerd Font Mono",
    "Symbols Nerd Font",
    "JetBrainsMono Nerd Font Mono",
    "MesloLGS NF",
    "Noto Color Emoji",
    "Noto Sans Symbols 2",
    "DejaVu Sans Mono",
];

pub(crate) fn base_font(settings: &TerminalSettings) -> Font {
    let mut base = font(settings.font_family.clone());
    base.fallbacks = Some(FontFallbacks::from_fonts(
        FALLBACKS
            .iter()
            .map(|family| (*family).to_owned())
            .collect(),
    ));
    base
}

pub(crate) fn styled_font(
    base: &Font,
    flags: StyleFlags,
) -> Font {
    let mut styled = base.clone();
    if flags.contains(StyleFlags::BOLD) {
        styled.weight = FontWeight::BOLD;
    }
    if flags.contains(StyleFlags::ITALIC) {
        styled.style = FontStyle::Italic;
    }
    styled
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn styles_apply_on_top_of_the_base_font() {
        let base = base_font(&TerminalSettings::default());
        assert_eq!(base.family.as_ref(), "Geist Mono");
        assert!(base.fallbacks.is_some());
        let bold_italic = styled_font(&base, StyleFlags::BOLD.with(StyleFlags::ITALIC));
        assert_eq!(
            (bold_italic.weight, bold_italic.style),
            (FontWeight::BOLD, FontStyle::Italic)
        );
        assert_eq!(styled_font(&base, StyleFlags::NONE), base);
    }
}
