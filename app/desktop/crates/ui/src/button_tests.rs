use std::cell::Cell;
use std::rc::Rc;

use gpui::{Context, Render, TestAppContext, VisualTestContext, Window, point, px};

use super::*;

struct Harness {
    clicks: Rc<Cell<u32>>,
    disabled: bool,
}

impl Render for Harness {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let clicks = Rc::clone(&self.clicks);
        Button::new("go", "Go")
            .disabled(self.disabled)
            .on_click(move |_, _, _| clicks.set(clicks.get() + 1))
    }
}

fn open(cx: &mut TestAppContext, disabled: bool) -> (Rc<Cell<u32>>, &mut VisualTestContext) {
    cx.update(|cx| {
        crate::init(cx).expect("init");
    });
    let clicks = Rc::new(Cell::new(0));
    let shared = Rc::clone(&clicks);
    let (_view, cx) = cx.add_window_view(move |_, _| Harness { clicks: shared, disabled });
    (clicks, cx)
}

#[gpui::test]
fn click_invokes_the_handler(cx: &mut TestAppContext) {
    let (clicks, cx) = open(cx, false);
    cx.simulate_click(point(px(20.0), px(16.0)), gpui::Modifiers::none());
    assert_eq!(clicks.get(), 1);
}

#[gpui::test]
fn disabled_button_ignores_clicks(cx: &mut TestAppContext) {
    let (clicks, cx) = open(cx, true);
    cx.simulate_click(point(px(20.0), px(16.0)), gpui::Modifiers::none());
    assert_eq!(clicks.get(), 0);
}
