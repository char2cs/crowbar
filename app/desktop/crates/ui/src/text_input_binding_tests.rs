use std::cell::Cell;
use std::rc::Rc;

use gpui::TestAppContext;

use crate::TextInputEvent;
use crate::test_support::field;

#[gpui::test]
fn enter_submits_and_escape_cancels(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    let (submitted, cancelled) = (Rc::new(Cell::new(0)), Rc::new(Cell::new(0)));
    let (on_submit, on_cancel) = (Rc::clone(&submitted), Rc::clone(&cancelled));
    cx.update(|_, cx| {
        cx.subscribe(&input, move |_, event: &TextInputEvent, _| match event {
            TextInputEvent::Submitted => on_submit.set(on_submit.get() + 1),
            TextInputEvent::Cancelled => on_cancel.set(on_cancel.get() + 1),
            TextInputEvent::Changed(_) => {},
        })
        .detach();
    });
    cx.simulate_keystrokes("enter escape escape");
    assert_eq!((submitted.get(), cancelled.get()), (1, 2));
}
