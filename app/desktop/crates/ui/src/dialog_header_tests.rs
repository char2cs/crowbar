use std::cell::Cell;
use std::rc::Rc;

use gpui::{Modifiers, TestAppContext, point, px};

use super::*;
use crate::test_support::open;

#[test]
fn emptiness_follows_the_parts() {
    assert!(DialogHeader::new(ElementId::from("h")).is_empty());
    assert!(
        !DialogHeader::new(ElementId::from("h"))
            .title(Some("Title".into()))
            .is_empty()
    );
    assert!(
        !DialogHeader::new(ElementId::from("h"))
            .icon(Some(IconName::Gear))
            .is_empty()
    );
}

#[gpui::test]
fn close_button_calls_the_handler(cx: &mut TestAppContext) {
    let closed = Rc::new(Cell::new(0));
    let sink = Rc::clone(&closed);
    let cx = open(cx, move |_, _| {
        let sink = Rc::clone(&sink);
        let handler: CloseHandler = Rc::new(move |_, _| sink.set(sink.get() + 1));
        DialogHeader::new(ElementId::from("h"))
            .title(Some("T".into()))
            .on_close(Some(handler))
    });
    let bounds = cx.debug_bounds("probe").unwrap_or_default();
    let button = point(bounds.right() - px(16.0 + 14.0), bounds.top() + px(14.0));
    cx.simulate_click(button, Modifiers::none());
    assert_eq!(closed.get(), 1);
}
