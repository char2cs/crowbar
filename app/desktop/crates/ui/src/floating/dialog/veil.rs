//! What a dialog paints beneath its card: the window behind, dimmed.

use gpui::{Animation, App, Div, ParentElement, Pixels, Size, Styled, div, hsla};

use crate::foundation::CubicBezier;
use crate::foundation::Motion;
use crate::foundation::tokens::timing;

const SCRIM: f32 = 0.32;

pub(super) fn layer(viewport: Size<Pixels>) -> Div {
    covering(viewport).child(covering(viewport).bg(hsla(0.0, 0.0, 0.0, SCRIM)))
}

pub(super) fn fade(cx: &App) -> Animation {
    Animation::new(Motion::of(cx).duration(timing::SLOW))
        .with_easing(|t| CubicBezier::DEFAULT.ease(t))
}

fn covering(viewport: Size<Pixels>) -> Div {
    div()
        .absolute()
        .top_0()
        .left_0()
        .w(viewport.width)
        .h(viewport.height)
}

#[cfg(test)]
mod tests {
    use gpui::{InteractiveElement, TestAppContext, px, size};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn the_veil_covers_the_viewport_it_is_given(cx: &mut TestAppContext) {
        let viewport = size(px(120.0), px(80.0));
        let window = open(cx, move |_, _| {
            layer(viewport).debug_selector(|| "veil".into())
        });
        let veil = window.debug_bounds("veil").unwrap_or_default();
        assert_eq!(veil.size, viewport);
    }
}
