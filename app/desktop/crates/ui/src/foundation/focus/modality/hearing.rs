//! What every window root listens to: pointer presses, for the input modality, and the
//! pointer leaving the window.

use gpui::{
    App, DispatchPhase, IntoElement, MouseExitEvent, MouseMoveEvent, PlatformInput, Styled, Window,
    canvas, point, px,
};

use crate::foundation::focus::modality::pressed;

const OUTSIDE: f32 = -1.0;

/// An empty element that hears the window it is painted in: a press there turns
/// [`crate::Modality`] to the pointer, and the pointer leaving it unhovers everything, as
/// a browser does. Every window root paints one.
pub fn hear() -> impl IntoElement {
    canvas(
        |_, _, _| {},
        |_, (), window, _| {
            window.on_mouse_event(pressed);
            window.on_mouse_event(left);
        },
    )
    .absolute()
    .size_0()
}

fn left(
    event: &MouseExitEvent,
    phase: DispatchPhase,
    window: &mut Window,
    cx: &mut App,
) {
    if phase != DispatchPhase::Bubble {
        return;
    }
    let away = PlatformInput::MouseMove(MouseMoveEvent {
        position: point(px(OUTSIDE), px(OUTSIDE)),
        pressed_button: event.pressed_button,
        modifiers: event.modifiers,
    });
    window.defer(cx, move |window, cx| {
        window.dispatch_event(away, cx);
    });
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{InteractiveElement as _, StatefulInteractiveElement as _, TestAppContext, div};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn the_pointer_leaving_the_window_unhovers_what_it_was_over(cx: &mut TestAppContext) {
        let hovered = Rc::new(Cell::new(false));
        let seen = Rc::clone(&hovered);
        let cx = open(cx, move |_, _| {
            let seen = Rc::clone(&seen);
            div()
                .id("spot")
                .size(px(40.0))
                .on_hover(move |hovered, _, _| seen.set(*hovered))
        });
        let inside = point(px(10.0), px(10.0));
        cx.simulate_mouse_move(inside, None, gpui::Modifiers::none());
        assert!(hovered.get(), "over it");
        cx.simulate_event(MouseExitEvent {
            position: inside,
            pressed_button: None,
            modifiers: gpui::Modifiers::none(),
        });
        cx.run_until_parked();
        assert!(!hovered.get(), "the pointer left the window");
    }
}
