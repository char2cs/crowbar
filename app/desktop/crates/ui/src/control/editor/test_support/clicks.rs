#![cfg(test)]

use gpui::{
    Modifiers, MouseButton, MouseDownEvent, MouseUpEvent, Pixels, Point, VisualTestContext,
};

fn down(
    at: Point<Pixels>,
    click_count: usize,
) -> MouseDownEvent {
    MouseDownEvent {
        button: MouseButton::Left,
        position: at,
        modifiers: Modifiers::none(),
        click_count,
        first_mouse: false,
    }
}

fn up(
    at: Point<Pixels>,
    click_count: usize,
) -> MouseUpEvent {
    MouseUpEvent {
        button: MouseButton::Left,
        position: at,
        modifiers: Modifiers::none(),
        click_count,
    }
}

pub(crate) fn press(
    cx: &mut VisualTestContext,
    at: Point<Pixels>,
    clicks: usize,
) {
    for count in 1..=clicks {
        cx.simulate_event(down(at, count));
        cx.simulate_event(up(at, count));
    }
}
