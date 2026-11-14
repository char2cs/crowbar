use gpui::TestAppContext;

use crate::test_support::{field, text_of};

#[gpui::test]
fn typing_inserts_at_the_caret(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("héllo");
    assert_eq!(text_of(&input, cx), "héllo");
}

#[gpui::test]
fn set_text_replaces_and_the_placeholder_is_kept(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    input.update(cx, |input, cx| input.set_text("query", cx));
    assert_eq!(text_of(&input, cx), "query");
    assert_eq!(
        input.read_with(cx, |input, _| input.placeholder().to_string()),
        "Search"
    );
}

#[gpui::test]
fn editing_notifies_subscribers_with_the_new_text(cx: &mut TestAppContext) {
    use std::cell::RefCell;
    use std::rc::Rc;

    use crate::TextInputEvent;
    let (input, cx) = field(cx);
    let seen = Rc::new(RefCell::new(Vec::new()));
    let sink = Rc::clone(&seen);
    cx.update(|_, cx| {
        cx.subscribe(&input, move |_, event: &TextInputEvent, _| {
            if let TextInputEvent::Changed(text) = event {
                sink.borrow_mut().push(text.clone());
            }
        })
        .detach();
    });
    cx.simulate_input("ab");
    assert_eq!(seen.borrow().last().map(String::as_str), Some("ab"));
}
