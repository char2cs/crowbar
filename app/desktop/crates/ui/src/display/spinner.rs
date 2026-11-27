use gpui::{
    AnimationExt, App, ElementId, Hsla, IntoElement, Pixels, RenderOnce, Styled, Transformation,
    Window, percentage, svg,
};

use crate::foundation::CubicBezier;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::IconWeight;
use crate::foundation::Motion;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::timing;

const OPACITY: f32 = 0.8;

#[derive(IntoElement)]
pub struct Spinner {
    id: ElementId,
    size: Pixels,
    surface: Surface,
    color: Option<Hsla>,
}

impl Spinner {
    pub fn new(id: impl Into<ElementId>) -> Self {
        Self {
            id: id.into(),
            size: spacing::STEP_4,
            surface: Surface::default(),
            color: None,
        }
    }

    #[must_use]
    pub fn size(
        mut self,
        size: Pixels,
    ) -> Self {
        self.size = size;
        self
    }

    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    #[must_use]
    pub fn color(
        mut self,
        color: Hsla,
    ) -> Self {
        self.color = Some(color);
        self
    }
}

impl RenderOnce for Spinner {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let muted = Theme::of(cx)
            .palette(self.surface)
            .muted_foreground()
            .opacity(OPACITY);
        let color = self.color.unwrap_or(muted);
        let path = Icon::path(IconName::CircleNotch, IconWeight::Regular).unwrap_or("");
        svg()
            .path(path)
            .size(self.size)
            .flex_none()
            .text_color(color)
            .with_animation(
                self.id,
                Motion::essential_loop(timing::SPIN, CubicBezier::LINEAR),
                |element, delta| {
                    element.with_transformation(Transformation::rotate(percentage(delta)))
                },
            )
    }
}

opaque_debug!(Spinner);

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::probe::measure;

    #[gpui::test]
    fn default_size_is_sixteen_pixels(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| Spinner::new("s"));
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (16.0, 16.0)
        );
        let big = measure(cx, |_, _| Spinner::new("s").size(px(24.0)));
        assert_eq!(f32::from(big.size.width), 24.0);
    }

    #[gpui::test]
    fn an_explicit_colour_does_not_change_the_size(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            Spinner::new("s").color(gpui::hsla(0.0, 0.0, 1.0, 1.0))
        });
        assert_eq!(f32::from(bounds.size.width), 16.0);
    }
}
