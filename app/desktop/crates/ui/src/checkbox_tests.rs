use std::cell::Cell;
use std::rc::Rc;

use gpui::{TestAppContext, point, px};

use super::*;
use crate::test_support::{measure, open};

#[gpui::test]
fn box_is_sixteen_pixels(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| Checkbox::new("c", false));
    assert_eq!((f32::from(bounds.size.width), f32::from(bounds.size.height)), (16.0, 16.0));
}

#[gpui::test]
fn click_requests_the_opposite_value(cx: &mut TestAppContext) {
    let seen = Rc::new(Cell::new(None));
    let sink = Rc::clone(&seen);
    let cx = open(cx, move |_, _| {
        let sink = Rc::clone(&sink);
        Checkbox::new("c", false).on_change(move |value, _, _| sink.set(Some(value)))
    });
    cx.simulate_click(point(px(8.0), px(8.0)), gpui::Modifiers::none());
    assert_eq!(seen.get(), Some(true));
}
