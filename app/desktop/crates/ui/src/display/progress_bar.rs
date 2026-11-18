//! A horizontal progress indicator, determinate or indeterminate.

use gpui::{
    AnimationExt, App, ElementId, InteractiveElement, IntoElement, ParentElement, Pixels,
    RenderOnce, Styled, Window, div, px, relative,
};

use crate::foundation::CubicBezier;
use crate::foundation::Motion;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::timing;

const HEIGHT: Pixels = px(6.0);
const DEFAULT_WIDTH: Pixels = px(240.0);
const SWEEP_SPAN: f32 = 0.4;
const SWEEP: std::time::Duration = std::time::Duration::from_millis(1400);

/// A rounded track with a primary fill; without a fraction a pill sweeps across it.
#[derive(IntoElement)]
pub struct ProgressBar {
    id: ElementId,
    fraction: Option<f32>,
    from: Option<f32>,
    width: Pixels,
    surface: Surface,
}

impl ProgressBar {
    /// A bar at `fraction` of its width (clamped to 0..=1), or indeterminate when `None`.
    pub fn new(
        id: impl Into<ElementId>,
        fraction: Option<f32>,
    ) -> Self {
        Self {
            id: id.into(),
            fraction: fraction.map(clamp_fraction),
            from: None,
            width: DEFAULT_WIDTH,
            surface: Surface::default(),
        }
    }

    /// Animates the fill from `previous` to the current fraction when the fraction changes.
    #[must_use]
    pub fn from(
        mut self,
        previous: f32,
    ) -> Self {
        self.from = Some(clamp_fraction(previous));
        self
    }

    /// Sets the track width.
    #[must_use]
    pub fn width(
        mut self,
        width: Pixels,
    ) -> Self {
        self.width = width;
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

fn clamp_fraction(fraction: f32) -> f32 {
    if fraction.is_nan() {
        return 0.0;
    }
    fraction.clamp(0.0, 1.0)
}

impl RenderOnce for ProgressBar {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(self.surface);
        let motion = Motion::of(cx);
        let track = div()
            .id(self.id.clone())
            .relative()
            .flex_none()
            .w(self.width)
            .h(HEIGHT)
            .overflow_hidden()
            .rounded(radius::FULL)
            .bg(palette.border());
        let fill = div()
            .absolute()
            .top_0()
            .bottom_0()
            .rounded(radius::FULL)
            .bg(palette.primary());
        let Some(target) = self.fraction else {
            return track.child(fill.w(relative(SWEEP_SPAN)).with_animation(
                (self.id, "sweep"),
                Motion::essential_loop(SWEEP, CubicBezier::DEFAULT),
                |fill, delta| fill.left(relative(delta * (1.0 + SWEEP_SPAN) - SWEEP_SPAN)),
            ));
        };
        let start = self.from.unwrap_or(target);
        let key = (self.id, format!("fill-{}", target.to_bits()));
        track.child(fill.left_0().with_animation(
            key,
            motion.animation(timing::SLOW, CubicBezier::DEFAULT),
            move |fill, delta| fill.w(relative(start + (target - start) * delta)),
        ))
    }
}

opaque_debug!(ProgressBar);

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::probe::measure;

    fn bar(fraction: Option<f32>) -> ProgressBar {
        ProgressBar::new("bar", fraction).width(px(120.0))
    }

    #[gpui::test]
    fn track_has_the_requested_width_and_a_fixed_height(cx: &mut TestAppContext) {
        for fraction in [None, Some(0.0), Some(0.5), Some(2.0)] {
            let bounds = measure(cx, move |_, _| bar(fraction));
            assert_eq!(
                (f32::from(bounds.size.width), f32::from(bounds.size.height)),
                (120.0, 6.0),
                "{fraction:?}"
            );
        }
    }

    #[test]
    fn fractions_are_clamped_and_nan_is_empty() {
        const CASES: &[(f32, f32)] = &[(-1.0, 0.0), (0.25, 0.25), (3.0, 1.0), (f32::NAN, 0.0)];
        for &(input, expected) in CASES {
            assert_eq!(clamp_fraction(input), expected, "{input}");
        }
    }
}
