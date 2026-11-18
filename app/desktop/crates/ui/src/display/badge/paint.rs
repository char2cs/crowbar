//! Colours of a badge variant.

use gpui::{Hsla, hsla};

use crate::display::badge::badge_variant::BadgeVariant;
use crate::foundation::Appearance;
use crate::foundation::Palette;

const TINT_LIGHT: f32 = 0.08;
const TINT_DARK: f32 = 0.16;
const OUTLINE_DARK_FILL: f32 = 0.32;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Paint {
    pub(crate) fill: Hsla,
    pub(crate) text: Hsla,
    pub(crate) border: Hsla,
}

impl Paint {
    pub(crate) fn of(
        variant: BadgeVariant,
        palette: &Palette,
        appearance: Appearance,
    ) -> Self {
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

    fn solid(
        fill: Hsla,
        text: Hsla,
    ) -> Self {
        Self {
            fill,
            text,
            border: hsla(0.0, 0.0, 0.0, 0.0),
        }
    }

    fn tinted(
        hue: Hsla,
        light_text: Hsla,
        appearance: Appearance,
    ) -> Self {
        let (alpha, text) = match appearance {
            Appearance::Light => (TINT_LIGHT, light_text),
            Appearance::Dark => (TINT_DARK, hue),
        };
        Self::solid(hue.opacity(alpha), text)
    }

    fn destructive(
        palette: &Palette,
        appearance: Appearance,
    ) -> Self {
        let text = match appearance {
            Appearance::Dark => palette.destructive_foreground(),
            Appearance::Light => hsla(0.0, 0.0, 1.0, 1.0),
        };
        Self::solid(palette.destructive(), text)
    }

    fn outline(
        palette: &Palette,
        appearance: Appearance,
    ) -> Self {
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
mod tests {
    use super::*;
    use crate::foundation::Surface;
    use crate::foundation::Theme;

    type Check = fn(&Paint, &Palette) -> bool;

    type Case = (&'static str, BadgeVariant, Appearance, Check);

    const CASES: &[Case] = &[
        (
            "tinted light fill is faint",
            BadgeVariant::Success,
            Appearance::Light,
            |paint, _| (paint.fill.a - TINT_LIGHT).abs() < 1e-6,
        ),
        (
            "tinted dark fill is stronger",
            BadgeVariant::Success,
            Appearance::Dark,
            |paint, _| (paint.fill.a - TINT_DARK).abs() < 1e-6,
        ),
        (
            "tinted dark text is the hue",
            BadgeVariant::Success,
            Appearance::Dark,
            |paint, palette| paint.text == palette.success(),
        ),
        (
            "solid fill is the colour",
            BadgeVariant::Primary,
            Appearance::Light,
            |paint, palette| paint.fill == palette.primary(),
        ),
        (
            "solid has no border",
            BadgeVariant::Primary,
            Appearance::Light,
            |paint, _| paint.border.a.abs() < f32::EPSILON,
        ),
        (
            "outline has a border",
            BadgeVariant::Outline,
            Appearance::Light,
            |paint, palette| paint.border == palette.input(),
        ),
    ];

    #[test]
    fn paints_follow_the_variant_and_appearance() {
        let palette = Theme::default().palette(Surface::Window);
        for &(name, variant, appearance, check) in CASES {
            let paint = Paint::of(variant, palette, appearance);
            assert!(check(&paint, palette), "{name}");
        }
    }
}
