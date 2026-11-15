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
    pub fn variant(
        mut self,
        variant: BadgeVariant,
    ) -> Self {
        self.variant = variant;
        self
    }

    /// Chooses the size step.
    #[must_use]
    pub fn size(
        mut self,
        size: BadgeSize,
    ) -> Self {
        self.size = size;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }
}

impl RenderOnce for Badge {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
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
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn height_follows_the_size_step(cx: &mut TestAppContext) {
        const CASES: &[(BadgeSize, f32)] = &[
            (BadgeSize::Sm, 16.0),
            (BadgeSize::Md, 18.0),
            (BadgeSize::Lg, 22.0),
        ];
        for &(size, height) in CASES {
            let bounds = measure(cx, move |_, _| Badge::new("3").size(size));
            assert_eq!(f32::from(bounds.size.height), height, "{size:?}");
            assert!(
                f32::from(bounds.size.width) >= height,
                "{size:?} keeps its minimum width"
            );
        }
    }

    #[gpui::test]
    fn every_variant_renders(cx: &mut TestAppContext) {
        for variant in [
            BadgeVariant::Primary,
            BadgeVariant::Destructive,
            BadgeVariant::Error,
            BadgeVariant::Info,
            BadgeVariant::Outline,
            BadgeVariant::Secondary,
            BadgeVariant::Success,
            BadgeVariant::Warning,
        ] {
            let bounds = measure(cx, move |_, _| Badge::new("x").variant(variant));
            assert!(f32::from(bounds.size.width) > 0.0, "{variant:?}");
        }
    }
}
