//! Colours of a badge variant.

use gpui::{Hsla, hsla};

use crate::appearance::Appearance;
use crate::badge_variant::BadgeVariant;
use crate::palette::Palette;

const TINT_LIGHT: f32 = 0.08;
const TINT_DARK: f32 = 0.16;
const OUTLINE_DARK_FILL: f32 = 0.32;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct BadgePaint {
    pub(crate) fill: Hsla,
    pub(crate) text: Hsla,
    pub(crate) border: Hsla,
}

impl BadgePaint {
    pub(crate) fn of(variant: BadgeVariant, palette: &Palette, appearance: Appearance) -> Self {
        match variant {
            BadgeVariant::Primary => Self::solid(palette.primary(), palette.primary_foreground()),
            BadgeVariant::Secondary => {
                Self::solid(palette.secondary(), palette.secondary_foreground())
            },
            BadgeVariant::Destructive => Self::destructive(palette, appearance),
            BadgeVariant::Outline => Self::outline(palette, appearance),
            BadgeVariant::Error => Self::tinted(
                palette.destructive(),
                palette.destructive_foreground(),
                appearance,
            ),
            BadgeVariant::Info => {
                Self::tinted(palette.info(), palette.info_foreground(), appearance)
            },
            BadgeVariant::Success => {
                Self::tinted(palette.success(), palette.success_foreground(), appearance)
            },
            BadgeVariant::Warning => {
                Self::tinted(palette.warning(), palette.warning_foreground(), appearance)
            },
        }
    }

    fn solid(fill: Hsla, text: Hsla) -> Self {
        Self {
            fill,
            text,
            border: hsla(0.0, 0.0, 0.0, 0.0),
        }
    }

    fn tinted(hue: Hsla, light_text: Hsla, appearance: Appearance) -> Self {
        let (alpha, text) = match appearance {
            Appearance::Light => (TINT_LIGHT, light_text),
            Appearance::Dark => (TINT_DARK, hue),
        };
        Self::solid(hue.opacity(alpha), text)
    }

    fn destructive(palette: &Palette, appearance: Appearance) -> Self {
        let text = match appearance {
            Appearance::Dark => palette.destructive_foreground(),
            Appearance::Light => hsla(0.0, 0.0, 1.0, 1.0),
        };
        Self::solid(palette.destructive(), text)
    }

    fn outline(palette: &Palette, appearance: Appearance) -> Self {
        let fill = match appearance {
            Appearance::Dark => palette.input().opacity(OUTLINE_DARK_FILL),
            Appearance::Light => palette.background(),
        };
        Self {
            fill,
            text: palette.foreground(),
            border: palette.input(),
        }
    }
}

#[cfg(test)]
#[path = "badge_paint_tests.rs"]
mod tests;
