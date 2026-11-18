//! Placement of popups below their trigger and the click-away layer behind them.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Corner, InteractiveElement, IntoElement, MouseButton, ParentElement, Pixels,
    Point, Styled, Window, anchored, deferred, div, point, px, relative,
};

use crate::floating::placement::Placement;
use crate::foundation::tokens::layer;

const WINDOW_MARGIN: f32 = 8.0;
const SUBMENU_GAP: f32 = 2.0;
const SUBMENU_RISE: f32 = 5.0;

pub(crate) fn placed(
    placement: Placement,
    offset: f32,
    content: impl IntoElement,
) -> AnyElement {
    match placement {
        Placement::Below => below(offset, content),
        Placement::BelowEnd => below_end(offset, content),
        Placement::BelowCentre => below_centre(offset, content),
        Placement::Right => right_of(offset, content),
    }
}

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

pub(crate) fn below_end(
    offset: f32,
    content: impl IntoElement,
) -> AnyElement {
    div()
        .absolute()
        .right_0()
        .top_full()
        .mt(px(offset))
        .child(
            deferred(
                anchored()
                    .anchor(Corner::TopRight)
                    .snap_to_window_with_margin(px(WINDOW_MARGIN))
                    .child(content),
            )
            .with_priority(layer::MENU),
        )
        .into_any_element()
}

pub(crate) fn below_centre(
    offset: f32,
    content: impl IntoElement,
) -> AnyElement {
    div()
        .absolute()
        .left(relative(0.5))
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

pub(crate) fn right_of(
    offset: f32,
    content: impl IntoElement,
) -> AnyElement {
    div()
        .absolute()
        .left_full()
        .top_0()
        .ml(px(offset))
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

/// A popup placed with its top-left corner at `position`.
pub fn popup_at(
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

/// A window-sized layer behind a popup; a press on it, with either button, runs `on_press`.
pub fn click_away(
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
    use crate::test_support::probe::open;

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
    fn content_is_laid_out_right_of_the_trigger_top_aligned(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div().relative().w(px(100.0)).h(px(30.0)).child(right_of(
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
            content.origin.x >= trigger.right() + px(4.0) - px(0.5),
            "{content:?} right of {trigger:?}"
        );
        assert!((content.origin.y - trigger.origin.y).abs() < px(0.5));
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

    #[gpui::test]
    fn an_end_aligned_popup_shares_the_right_edge_of_its_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div().relative().w(px(100.0)).h(px(30.0)).child(below_end(
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
            (content.right() - trigger.right()).abs() < px(0.5),
            "{content:?} end of {trigger:?}"
        );
        assert!(content.origin.y >= trigger.bottom() + px(4.0) - px(0.5));
    }

    #[gpui::test]
    fn a_centred_popup_starts_at_the_horizontal_centre_of_its_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div()
                .relative()
                .w(px(100.0))
                .h(px(30.0))
                .child(below_centre(
                    4.0,
                    div()
                        .w(px(80.0))
                        .h(px(20.0))
                        .debug_selector(|| "popup-content".into()),
                ))
        });
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let content = cx.debug_bounds("popup-content").unwrap_or_default();
        let centre = trigger.origin.x + trigger.size.width * 0.5;
        assert!(
            (content.origin.x - centre).abs() < px(0.5),
            "{content:?} from {centre:?}"
        );
        assert!((content.origin.y - (trigger.bottom() + px(4.0))).abs() < px(0.5));
    }
}
