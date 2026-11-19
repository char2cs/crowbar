//! What a dialog paints beneath its card: the window behind, dimmed and blurred.

use gpui::{
    Animation, AnyElement, App, Div, ElementId, ParentElement, Pixels, Size, Styled, Window, div,
    hsla, px,
};

use crate::floating::Backdrop;
use crate::foundation::CubicBezier;
use crate::foundation::Motion;
use crate::foundation::tokens::timing;

const SCRIM: f32 = 0.32;
const BACKDROP_BLUR: f32 = 4.0;

pub(super) fn layers(
    id: (ElementId, &'static str),
    viewport: Size<Pixels>,
    window: &mut Window,
    cx: &mut App,
) -> (Option<AnyElement>, Div) {
    let backdrop = Backdrop::of(id, window, cx);
    let (still, blurred) = backdrop.update(cx, |backdrop, _| {
        (
            backdrop.plain(window),
            backdrop.blurred(px(BACKDROP_BLUR), window),
        )
    });
    let veil = covering(viewport)
        .children(blurred)
        .child(covering(viewport).bg(hsla(0.0, 0.0, 0.0, SCRIM)));
    (still, veil)
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
    use gpui::TestAppContext;

    use crate::Dialog;
    use crate::foundation::Motion;
    use crate::test_support::probe::open_floating;

    #[gpui::test]
    fn without_a_compositor_the_window_beneath_shows_under_the_scrim(cx: &mut TestAppContext) {
        cx.update(|cx| cx.set_global(Motion::reduced()));
        let (mut owner, mut top) = open_floating(cx, |_, _| {
            crate::floating::Portal::window("dialog", |_, _| Dialog::new("d").title("Rename"))
        });
        owner.update(|window, _| window.refresh());
        owner.run_until_parked();
        let whole = top
            .update(|window, _| window.owner_bounds())
            .unwrap_or_default();
        let content = top.update(|window, _| window.content_region().to_vec());
        assert!(
            content.contains(&whole),
            "the whole window shows: {content:?}"
        );
    }
}
