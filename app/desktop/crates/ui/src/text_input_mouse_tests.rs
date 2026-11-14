use gpui::{Modifiers, TestAppContext, point, px};

use crate::test_support::{field, text_of};

#[gpui::test]
fn clicking_past_the_text_puts_the_caret_at_its_end(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("abc");
    cx.simulate_keystrokes("home");
    cx.simulate_click(point(px(400.0), px(16.0)), Modifiers::none());
    cx.simulate_input("!");
    assert_eq!(text_of(&input, cx), "abc!");
}

#[gpui::test]
fn clicking_at_the_left_edge_puts_the_caret_at_the_start(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("abc");
    cx.simulate_click(point(px(11.0), px(16.0)), Modifiers::none());
    cx.simulate_input("!");
    assert_eq!(text_of(&input, cx), "!abc");
}
