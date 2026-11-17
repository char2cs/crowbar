//! Placeholder block with a viewport-fixed travelling highlight.

use std::ops::Range;

use gpui::{
    App, Bounds, ElementId, Hsla, InteractiveElement as _, IntoElement, ParentElement as _, Pixels,
    RenderOnce, Styled as _, Window, canvas, div, fill, hsla, linear_color_stop, linear_gradient,
    point, size,
};

use crate::appearance::Appearance;
use crate::opaque_debug::opaque_debug;
use crate::radius;
use crate::shimmer_clock::ShimmerClock;
use crate::skeleton_stripe::stripes;
use crate::stripe::Stripe;
use crate::surface::Surface;
use crate::theme::Theme;

const LIGHT_HIGHLIGHT: f32 = 0.64;
const DARK_HIGHLIGHT: f32 = 0.04;
const GRADIENT_ANGLE: f32 = 90.0;

/// A muted rounded block whose highlight sweeps across the window every two seconds.
///
/// Like the CSS `background-attachment: fixed` of the React original, the highlight belongs to the
/// window, not to the block: every skeleton on screen shows the same stripe, tilted 120 degrees, and
/// the cycle starts one second in.
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

fn paint_half(
    window: &mut Window,
    span: &Range<Pixels>,
    rows: Range<Pixels>,
    shades: Range<Hsla>,
) {
    let area = Bounds::new(
        point(span.start, rows.start),
        size(span.end - span.start, rows.end - rows.start),
    );
    let gradient = linear_gradient(
        GRADIENT_ANGLE,
        linear_color_stop(shades.start, 0.0),
        linear_color_stop(shades.end, 1.0),
    );
    window.paint_quad(fill(area, gradient));
}

fn paint_stripe(
    window: &mut Window,
    stripe: &Stripe,
    rows: Range<Pixels>,
    glow: Hsla,
) {
    let clear = hsla(0.0, 0.0, 1.0, 0.0);
    paint_half(window, &stripe.rises(), rows.clone(), clear..glow);
    paint_half(window, &stripe.falls(), rows, glow..clear);
}

fn shimmer(
    bounds: Bounds<Pixels>,
    glow: Hsla,
    window: &mut Window,
    cx: &mut App,
) {
    let phase = ShimmerClock::of(cx).phase();
    let row = bounds.center().y;
    for stripe in stripes(phase, window.viewport_size(), row) {
        paint_stripe(window, &stripe, bounds.top()..bounds.bottom(), glow);
    }
    window.request_animation_frame();
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
        let glow = hsla(0.0, 0.0, 1.0, alpha);
        let band = canvas(
            |_, _, _| (),
            move |bounds, (), window, cx| shimmer(bounds, glow, window, cx),
        )
        .absolute()
        .size_full();
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

    #[gpui::test]
    fn block_has_the_requested_size(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| Skeleton::new("sk", px(120.0), px(14.0)));
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (120.0, 14.0)
        );
    }
}
