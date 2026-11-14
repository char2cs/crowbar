//! Small status and count pills.

use gpui::{
    App, FontWeight, Hsla, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, hsla,
};

use crate::appearance::Appearance;
use crate::badge_size::BadgeSize;
use crate::badge_variant::BadgeVariant;
use crate::font::UI_FAMILY;
use crate::palette::Palette;
use crate::surface::Surface;
use crate::theme::Theme;

const TINT_LIGHT: f32 = 0.08;
const TINT_DARK: f32 = 0.16;

/// A pill with a short label or count.
#[derive(Debug, Clone, IntoElement)]
pub struct Badge {
    label: SharedString,
    variant: BadgeVariant,
    size: BadgeSize,
    surface: Surface,
}

impl Badge {
    /// A primary, default-size badge.
    pub fn new(label: impl Into<SharedString>) -> Self {
        Self {
            label: label.into(),
            variant: BadgeVariant::default(),
            size: BadgeSize::default(),
            surface: Surface::default(),
        }
    }

    /// Chooses the colour variant.
    #[must_use]
    pub fn variant(mut self, variant: BadgeVariant) -> Self {
        self.variant = variant;
        self
    }

    /// Chooses the size step.
    #[must_use]
    pub fn size(mut self, size: BadgeSize) -> Self {
        self.size = size;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }
}

struct Paint {
    fill: Hsla,
    text: Hsla,
    border: Hsla,
}

fn tinted(fill: Hsla, text: Hsla, appearance: Appearance) -> Paint {
    let alpha = match appearance {
        Appearance::Light => TINT_LIGHT,
        Appearance::Dark => TINT_DARK,
    };
    Paint { fill: fill.opacity(alpha), text, border: hsla(0.0, 0.0, 0.0, 0.0) }
}

fn solid(fill: Hsla, text: Hsla) -> Paint {
    Paint { fill, text, border: hsla(0.0, 0.0, 0.0, 0.0) }
}

fn paint(variant: BadgeVariant, palette: &Palette, appearance: Appearance) -> Paint {
    let dark = appearance == Appearance::Dark;
    let tint_text = |foreground: Hsla, hue: Hsla| if dark { hue } else { foreground };
    match variant {
        BadgeVariant::Primary => solid(palette.primary(), palette.primary_foreground()),
        BadgeVariant::Secondary => solid(palette.secondary(), palette.secondary_foreground()),
        BadgeVariant::Destructive => {
            let text = if dark { palette.destructive_foreground() } else { hsla(0.0, 0.0, 1.0, 1.0) };
            solid(palette.destructive(), text)
        }
        BadgeVariant::Error => {
            tinted(palette.destructive(), tint_text(palette.destructive_foreground(), palette.destructive()), appearance)
        }
        BadgeVariant::Info => tinted(palette.info(), palette.info_foreground(), appearance),
        BadgeVariant::Success => {
            tinted(palette.success(), tint_text(palette.success_foreground(), palette.success()), appearance)
        }
        BadgeVariant::Warning => {
            tinted(palette.warning(), tint_text(palette.warning_foreground(), palette.warning()), appearance)
        }
        BadgeVariant::Outline => {
            let fill = if dark { palette.input().opacity(0.32) } else { palette.background() };
            Paint { fill, text: palette.foreground(), border: palette.input() }
        }
    }
}

impl RenderOnce for Badge {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let paint = paint(self.variant, theme.palette(self.surface), theme.appearance());
        div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .h(self.size.extent())
            .min_w(self.size.extent())
            .px(self.size.padding_x())
            .rounded(self.size.radius())
            .border_1()
            .border_color(paint.border)
            .bg(paint.fill)
            .text_color(paint.text)
            .text_size(self.size.text())
            .font_family(UI_FAMILY)
            .font_weight(FontWeight::MEDIUM)
            .whitespace_nowrap()
            .child(self.label)
    }
}

#[cfg(test)]
#[path = "badge_tests.rs"]
mod tests;
