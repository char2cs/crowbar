//! Placement of popups below their trigger and the click-away layer behind them.

use std::rc::Rc;

use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, MouseButton, ParentElement, Pixels, Point,
    Styled, Window, anchored, deferred, div, point, px,
};

use crate::layer;

const WINDOW_MARGIN: f32 = 8.0;
const SUBMENU_GAP: f32 = 2.0;
const SUBMENU_RISE: f32 = 5.0;

pub(crate) fn below(
    offset: f32,
    content: impl IntoElement,
) -> AnyElement {
    div()
        .absolute()
        .left_0()
        .top_full()
        .mt(px(offset))
        .child(
            deferred(
                anchored()
                    .snap_to_window_with_margin(px(WINDOW_MARGIN))
                    .child(content),
            )
            .with_priority(layer::MENU),
        )
        .into_any_element()
}

pub(crate) fn beside(content: impl IntoElement) -> AnyElement {
    div()
        .absolute()
        .left_full()
        .top(px(-SUBMENU_RISE))
        .ml(px(SUBMENU_GAP))
        .child(
            anchored()
                .snap_to_window_with_margin(px(WINDOW_MARGIN))
                .child(content),
        )
        .into_any_element()
}

pub(crate) fn at(
    position: Point<Pixels>,
    content: impl IntoElement,
) -> AnyElement {
    deferred(
        anchored()
            .position(position)
            .snap_to_window_with_margin(px(WINDOW_MARGIN))
            .child(content),
    )
    .with_priority(layer::MENU)
    .into_any_element()
}

pub(crate) fn click_away(
    window: &Window,
    on_press: impl Fn(&mut Window, &mut App) + 'static,
) -> AnyElement {
    let viewport = window.viewport_size();
    let on_press = Rc::new(on_press);
    let on_right = Rc::clone(&on_press);
    let layer = div()
        .occlude()
        .w(viewport.width)
        .h(viewport.height)
        .on_mouse_down(MouseButton::Left, move |_, window, cx| on_press(window, cx))
        .on_mouse_down(MouseButton::Right, move |_, window, cx| {
            on_right(window, cx);
        });
    deferred(anchored().position(point(px(0.0), px(0.0))).child(layer))
        .with_priority(layer::MENU - 1)
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::test_support::open;

    fn tally(counter: &Rc<Cell<u32>>) -> impl Fn(&mut Window, &mut App) + 'static {
        let counter = Rc::clone(counter);
        move |_, _| counter.set(counter.get() + 1)
    }

    #[gpui::test]
    fn press_anywhere_runs_the_close_handler_once_per_press(cx: &mut TestAppContext) {
        let presses = Rc::new(Cell::new(0));
        let counter = Rc::clone(&presses);
        let cx = open(cx, move |window, _| {
            let counter = Rc::clone(&counter);
            div()
                .relative()
                .w(px(40.0))
                .h(px(20.0))
                .child(click_away(window, tally(&counter)))
        });
        cx.simulate_click(point(px(300.0), px(300.0)), gpui::Modifiers::none());
        assert_eq!(presses.get(), 1);
        cx.simulate_click(point(px(5.0), px(5.0)), gpui::Modifiers::none());
        assert_eq!(presses.get(), 2);
    }

    #[gpui::test]
    fn content_is_laid_out_below_the_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div().relative().w(px(100.0)).h(px(30.0)).child(below(
                4.0,
                div()
                    .w(px(80.0))
                    .h(px(20.0))
                    .debug_selector(|| "popup-content".into()),
            ))
        });
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let content = cx.debug_bounds("popup-content").unwrap_or_default();
        assert!(
            content.origin.y >= trigger.bottom() + px(4.0) - px(0.5),
            "{content:?} below {trigger:?}"
        );
    }
}
