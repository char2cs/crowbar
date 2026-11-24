//! What a toast paints under its card while the card is translucent.

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnimationElement, AnimationExt, App, Div, ElementId, Entity, ParentElement, Styled, Window,
    canvas, div,
};

use crate::display::toast::motion::fade;
use crate::display::toast::phase::Phase;
use crate::floating::Backdrop;
use crate::foundation::CubicBezier;
use crate::foundation::Motion;
use crate::foundation::tokens::timing;

// PLATFORM: linux X11 without a compositor shows only opaque pixels; paint the window under a fade.
pub(crate) fn beneath(
    id: &ElementId,
    phase: Phase,
    window: &mut Window,
    cx: &mut App,
) -> AnimationElement<Div> {
    let backdrop = Backdrop::of((id.clone(), "beneath"), window, cx);
    div().absolute().inset_0().with_animation(
        (id.clone(), format!("beneath-{phase:?}")),
        Motion::of(cx).animation(timing::SLOW, CubicBezier::DEFAULT),
        move |slot, delta| slot.when(fade(phase, delta) < 1.0, |slot| capture(slot, &backdrop)),
    )
}

fn capture(
    slot: Div,
    backdrop: &Entity<Backdrop>,
) -> Div {
    let backdrop = backdrop.clone();
    let paint = move |bounds, (), window: &mut Window, cx: &mut App| {
        backdrop.update(cx, |backdrop, _| backdrop.paint_under(bounds, window));
    };
    slot.child(canvas(|_, _, _| (), paint).size_full())
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement, Styled, TestAppContext, div, px};

    use crate::display::toast::{Toast, ToastKind};
    use crate::floating::Portal;
    use crate::test_support::probe::open_floating;

    const TOAST_WIDTH: f32 = 288.0;

    fn sidebar_toast() -> gpui::Div {
        div()
            .w(px(TOAST_WIDTH))
            .child(Toast::new("t", ToastKind::Info, "Saved").full_width())
    }

    #[gpui::test]
    fn a_toast_fading_in_shows_over_the_window_beneath_without_a_compositor(
        cx: &mut TestAppContext
    ) {
        let (_owner, mut top) = open_floating(cx, |_, _| Portal::here("p", |_, _| sidebar_toast()));
        let card = top.debug_bounds("ui-toast").unwrap_or_default();
        let region = top.update(|window, _| window.content_region().to_vec());
        assert!(
            region.iter().any(|shown| shown.contains(&card.center())),
            "the translucent card shows, blended over the captured window: {region:?}"
        );
    }
}
