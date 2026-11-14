//! The rotating loading indicator.

use gpui::{
    Animation, AnimationExt, App, ElementId, IntoElement, Pixels, RenderOnce, Transformation, Window, percentage,
    px, svg, Styled,
};

use crate::cubic_bezier::CubicBezier;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;
use crate::motion::Motion;
use crate::theme::Theme;
use crate::surface::Surface;
use crate::timing;

const OPACITY: f32 = 0.8;

/// A `CircleNotch` turning once a second; it keeps spinning under reduced motion.
#[derive(IntoElement)]
pub struct Spinner {
    id: ElementId,
    size: Pixels,
    surface: Surface,
}

impl Spinner {
    /// A 16px spinner.
    pub fn new(id: impl Into<ElementId>) -> Self {
        Self { id: id.into(), size: px(16.0), surface: Surface::default() }
    }

    /// Sets the edge length.
    #[must_use]
    pub fn size(mut self, size: Pixels) -> Self {
        self.size = size;
        self
    }

    /// Takes the muted text colour of `surface`'s palette.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }
}

impl RenderOnce for Spinner {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let color = Theme::of(cx).palette(self.surface).muted_foreground().opacity(OPACITY);
        let path = Icon::path(IconName::CircleNotch, IconWeight::Regular).unwrap_or("");
        svg().path(path).size(self.size).flex_none().text_color(color).with_animation(
            self.id,
            Motion::essential_loop(timing::SPIN, CubicBezier::LINEAR),
            |element, delta| element.with_transformation(Transformation::rotate(percentage(delta))),
        )
    }
}


#[cfg(test)]
#[path = "spinner_tests.rs"]
mod tests;
