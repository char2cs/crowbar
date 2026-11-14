//! Placeholder block with a travelling highlight.

use gpui::{
    AnimationExt, App, ElementId, Hsla, IntoElement, ParentElement, Pixels, RenderOnce, Styled, Window, div, hsla,
    linear_color_stop, linear_gradient, relative,
};

use crate::appearance::Appearance;
use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;
use crate::radius;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::timing;

const BAND: f32 = 0.5;
const LIGHT_HIGHLIGHT: f32 = 0.64;
const DARK_HIGHLIGHT: f32 = 0.04;
const GRADIENT_ANGLE: f32 = 90.0;

/// A muted rounded block whose highlight sweeps across it every two seconds.
#[derive(IntoElement)]
pub struct Skeleton {
    id: ElementId,
    width: Pixels,
    height: Pixels,
}

impl Skeleton {
    /// A block of the given size.
    pub fn new(id: impl Into<ElementId>, width: Pixels, height: Pixels) -> Self {
        Self { id: id.into(), width, height }
    }
}

fn half(from: Hsla, to: Hsla) -> impl IntoElement {
    div()
        .h_full()
        .w(relative(BAND))
        .bg(linear_gradient(GRADIENT_ANGLE, linear_color_stop(from, 0.0), linear_color_stop(to, 1.0)))
}

impl RenderOnce for Skeleton {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let alpha = match theme.appearance() {
            Appearance::Light => LIGHT_HIGHLIGHT,
            Appearance::Dark => DARK_HIGHLIGHT,
        };
        let (clear, glow) = (hsla(0.0, 0.0, 1.0, 0.0), hsla(0.0, 0.0, 1.0, alpha));
        let band = div()
            .id((self.id.clone(), "band"))
            .absolute()
            .top_0()
            .h_full()
            .w(relative(BAND))
            .flex()
            .child(half(clear, glow))
            .child(half(glow, clear))
            .with_animation(
                self.id.clone(),
                Motion::essential_loop(timing::SKELETON, CubicBezier::LINEAR),
                |band, delta| band.left(relative(delta * (1.0 + BAND) - BAND)),
            );
        div()
            .id(self.id)
            .relative()
            .overflow_hidden()
            .flex_none()
            .w(self.width)
            .h(self.height)
            .rounded(radius::SM)
            .bg(theme.palette(Surface::Window).muted())
            .child(band)
    }
}

#[cfg(test)]
#[path = "skeleton_tests.rs"]
mod tests;
