//! Small status and count pills.

use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div,
};

use crate::badge_paint::BadgePaint;
use crate::badge_size::BadgeSize;
use crate::badge_variant::BadgeVariant;
use crate::font::UI_FAMILY;
use crate::surface::Surface;
use crate::theme::Theme;

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

impl RenderOnce for Badge {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let paint = BadgePaint::of(
            self.variant,
            theme.palette(self.surface),
            theme.appearance(),
        );
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
