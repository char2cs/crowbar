use std::cell::Cell;
use std::rc::Rc;

use gpui::{Modifiers, TestAppContext, point, px};

use super::*;
use crate::test_support::{measure, open};

#[gpui::test]
fn click_invokes_the_handler(cx: &mut TestAppContext) {
    let clicks = Rc::new(Cell::new(0));
    let sink = Rc::clone(&clicks);
    let cx = open(cx, move |_, _| {
        let sink = Rc::clone(&sink);
        Button::new("go", "Go").on_click(move |_, _, _| sink.set(sink.get() + 1))
    });
    cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
    assert_eq!(clicks.get(), 1);
}

#[gpui::test]
fn disabled_button_ignores_clicks(cx: &mut TestAppContext) {
    let clicks = Rc::new(Cell::new(0));
    let sink = Rc::clone(&clicks);
    let cx = open(cx, move |_, _| {
        let sink = Rc::clone(&sink);
        Button::new("go", "Go")
            .disabled()
            .on_click(move |_, _, _| sink.set(sink.get() + 1))
    });
    cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
    assert_eq!(clicks.get(), 0);
}

#[gpui::test]
fn sizes_set_the_height_and_icon_buttons_are_square(cx: &mut TestAppContext) {
    for size in [
        ButtonSize::Xs,
        ButtonSize::Sm,
        ButtonSize::Md,
        ButtonSize::Lg,
        ButtonSize::Xl,
    ] {
        let labelled = measure(cx, move |_, _| Button::new("a", "Go").size(size));
        let icon = measure(cx, move |_, _| {
            Button::icon_only("b", IconName::Gear).size(size)
        });
        assert_eq!(labelled.size.height, size.height(), "{size:?}");
        assert_eq!(icon.size.width, icon.size.height, "{size:?}");
    }
}
