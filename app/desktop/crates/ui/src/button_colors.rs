//! Resolved colours of one button variant.

use gpui::{BoxShadow, Hsla, hsla};

use crate::appearance::Appearance;
use crate::button_variant::ButtonVariant;
use crate::palette::Palette;
use crate::shadow;

const HOVER_FILL: f32 = 0.9;
const GLOW: f32 = 0.24;

/// Colours and shadow of a button in its rest and hover states.
#[derive(Debug, Clone)]
pub(crate) struct ButtonColors {
    pub(crate) background: Hsla,
    pub(crate) hover_background: Hsla,
    pub(crate) border: Hsla,
    pub(crate) foreground: Hsla,
    pub(crate) shadow: Vec<BoxShadow>,
}

fn transparent() -> Hsla {
    hsla(0.0, 0.0, 0.0, 0.0)
}

fn white() -> Hsla {
    hsla(0.0, 0.0, 1.0, 1.0)
}

impl ButtonColors {
    pub(crate) fn resolve(variant: ButtonVariant, palette: &Palette, appearance: Appearance) -> Self {
        match variant {
            ButtonVariant::Primary => Self::filled(palette.primary(), palette.primary_foreground()),
            ButtonVariant::Destructive => Self::filled(palette.destructive(), destructive_text(palette, appearance)),
            ButtonVariant::Secondary => Self::plain(palette.secondary(), palette.secondary_foreground()),
            ButtonVariant::Outline => Self::outline(palette, appearance),
            ButtonVariant::Ghost => Self {
                background: transparent(),
                hover_background: palette.accent(),
                border: transparent(),
                foreground: palette.foreground(),
                shadow: Vec::new(),
            },
        }
    }

    fn filled(fill: Hsla, foreground: Hsla) -> Self {
        Self {
            shadow: shadow::xs_tinted(fill.opacity(GLOW)),
            ..Self::plain(fill, foreground)
        }
    }

    fn plain(fill: Hsla, foreground: Hsla) -> Self {
        Self {
            background: fill,
            hover_background: fill.opacity(HOVER_FILL),
            border: fill,
            foreground,
            shadow: Vec::new(),
        }
    }

    fn outline(palette: &Palette, appearance: Appearance) -> Self {
        let (background, hover_background) = match appearance {
            Appearance::Light => (palette.popover(), palette.accent().opacity(0.5)),
            Appearance::Dark => (palette.input().opacity(0.32), palette.input().opacity(0.64)),
        };
        Self {
            background,
            hover_background,
            border: palette.input(),
            foreground: palette.foreground(),
            shadow: shadow::xs(0.05),
        }
    }
}

fn destructive_text(palette: &Palette, appearance: Appearance) -> Hsla {
    match appearance {
        Appearance::Light => white(),
        Appearance::Dark => palette.destructive_foreground(),
    }
}

#[cfg(test)]
#[path = "button_colors_tests.rs"]
mod tests;
