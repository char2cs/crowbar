use std::cell::Cell;
use std::rc::Rc;

use gpui::{TestAppContext, point, px};

use super::*;
use crate::test_support::open;

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
            .child(click_away(window, move |_, _| {
                counter.set(counter.get() + 1);
            }))
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
