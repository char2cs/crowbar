//! What a toast paints under its card while the card is translucent.

use gpui::{
    AnimationElement, AnimationExt, App, Bounds, Corners, Div, ElementId, Entity, ParentElement,
    Pixels, Styled, Window, canvas, div,
};

use crate::display::toast::motion::fade;
use crate::display::toast::phase::Phase;
use crate::floating::Backdrop;
use crate::foundation::CubicBezier;
use crate::foundation::Motion;
use crate::foundation::tokens::radius;
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
        move |slot, delta| reveal(slot, &backdrop, fade(phase, delta)),
    )
}

fn reveal(
    slot: Div,
    backdrop: &Entity<Backdrop>,
    opacity: f32,
) -> Div {
    if opacity <= 0.0 || opacity >= 1.0 {
        return slot;
    }
    let backdrop = backdrop.clone();
    let paint = move |bounds, (), window: &mut Window, cx: &mut App| {
        backdrop.update(cx, |backdrop, _| under_card(backdrop, bounds, window));
    };
    slot.child(canvas(|_, _, _| (), paint).size_full())
}

fn under_card(
    backdrop: &mut Backdrop,
    bounds: Bounds<Pixels>,
    window: &mut Window,
) {
    backdrop.paint_under(bounds, Corners::all(radius::LG), window);
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement, Styled, TestAppContext, VisualTestContext, div, px};

    use crate::display::toast::{Toast, ToastKind};
    use crate::floating::Portal;
    use crate::foundation::tokens::timing;
    use crate::test_support::probe::open_floating;

    const TOAST_WIDTH: f32 = 288.0;
    const CORNER: f32 = 0.5;

    fn sidebar_toast() -> gpui::Div {
        div()
            .w(px(TOAST_WIDTH))
            .child(Toast::new("t", ToastKind::Info, "Saved").full_width())
    }

    fn shown_at(
        top: &mut VisualTestContext,
        x: f32,
        y: f32,
    ) -> bool {
        let card = top.debug_bounds("ui-toast").unwrap_or_default();
        let at = gpui::point(card.left() + px(x), card.top() + px(y));
        let region = top.update(|window, _| window.content_region().to_vec());
        region.iter().any(|shown| shown.contains(&at))
    }

    fn shown(top: &mut VisualTestContext) -> bool {
        let card = top.debug_bounds("ui-toast").unwrap_or_default();
        let middle = card.size.height / 2.0;
        shown_at(top, TOAST_WIDTH / 2.0, f32::from(middle))
    }

    #[gpui::test]
    fn a_toast_fading_in_shows_over_the_window_beneath_without_a_compositor(
        cx: &mut TestAppContext
    ) {
        let (_owner, mut top) = open_floating(cx, |_, _| Portal::here("p", |_, _| sidebar_toast()));
        assert!(!shown(&mut top), "fully transparent: nothing to show");
        top.executor().advance_clock(timing::SLOW / 2);
        top.update(|window, cx| {
            window.refresh();
            window.draw(cx).clear(cx);
        });
        assert!(
            shown(&mut top),
            "half faded in: the card shows, blended over the captured window"
        );
        assert!(
            !shown_at(&mut top, CORNER, CORNER),
            "the capture keeps to the card's rounded corners"
        );
    }
}
