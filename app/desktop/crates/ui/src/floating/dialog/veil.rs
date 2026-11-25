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
