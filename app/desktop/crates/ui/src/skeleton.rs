//! Placeholder block with a travelling highlight.

use gpui::InteractiveElement as _;
use gpui::{
    AnimationExt, App, ElementId, Hsla, IntoElement, ParentElement, Pixels, RenderOnce, Styled,
    Window, div, hsla, linear_color_stop, linear_gradient, relative,
};

use crate::appearance::Appearance;
use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;
use crate::opaque_debug::opaque_debug;
use crate::radius;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::timing;

const BAND: f32 = 0.4;
const IDLE: f32 = 0.3;
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
    pub fn new(
        id: impl Into<ElementId>,
        width: Pixels,
        height: Pixels,
    ) -> Self {
        Self {
            id: id.into(),
            width,
            height,
        }
    }
}

fn half(
    from: Hsla,
    to: Hsla,
) -> impl IntoElement {
    div().h_full().w(relative(BAND)).bg(linear_gradient(
        GRADIENT_ANGLE,
        linear_color_stop(from, 0.0),
        linear_color_stop(to, 1.0),
    ))
}

fn band_left(delta: f32) -> f32 {
    if delta < IDLE {
        return 1.0;
    }
    -BAND + (delta - IDLE) / (1.0 - IDLE) * (1.0 + BAND)
}

impl RenderOnce for Skeleton {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
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
                |band, delta| band.left(relative(band_left(delta))),
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

opaque_debug!(Skeleton);

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::measure;

    #[test]
    fn the_highlight_rests_then_crosses_in_seventy_percent_of_the_cycle() {
        const CASES: &[(f32, f32)] = &[
            (0.0, 1.0),
            (0.29, 1.0),
            (0.3, -0.4),
            (0.65, 0.3),
            (1.0, 1.0),
        ];
        for &(delta, left) in CASES {
            assert!((band_left(delta) - left).abs() < 1e-5, "{delta}");
        }
    }

    #[gpui::test]
    fn block_has_the_requested_size(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| Skeleton::new("sk", px(120.0), px(14.0)));
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (120.0, 14.0)
        );
    }
}
