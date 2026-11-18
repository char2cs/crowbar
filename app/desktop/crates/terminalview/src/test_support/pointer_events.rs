use gpui::{Modifiers, MouseButton, MouseDownEvent, MouseUpEvent, Pixels, Point};

pub(crate) fn press(
    position: Point<Pixels>,
    modifiers: Modifiers,
) -> MouseDownEvent {
    MouseDownEvent {
        button: MouseButton::Left,
        position,
        modifiers,
        click_count: 1,
        first_mouse: false,
    }
}

pub(crate) fn release(
    position: Point<Pixels>,
    modifiers: Modifiers,
) -> MouseUpEvent {
    MouseUpEvent {
        button: MouseButton::Left,
        position,
        modifiers,
        click_count: 1,
    }
}
