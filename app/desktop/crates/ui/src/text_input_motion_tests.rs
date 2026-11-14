use gpui::TestAppContext;

use crate::test_support::{field, text_of};

#[gpui::test]
fn backspace_removes_a_whole_grapheme(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("ae\u{301}");
    cx.simulate_keystrokes("backspace");
    assert_eq!(text_of(&input, cx), "a");
}

#[gpui::test]
fn delete_removes_the_grapheme_after_the_caret(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("abc");
    cx.simulate_keystrokes("home delete");
    assert_eq!(text_of(&input, cx), "bc");
}

#[gpui::test]
fn arrows_move_the_caret_and_selection_collapses_to_its_edge(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("abc");
    cx.simulate_keystrokes("left left x");
    assert_eq!(text_of(&input, cx), "axbc");
    cx.simulate_keystrokes("shift-right shift-right left y");
    assert_eq!(text_of(&input, cx), "axybc");
}

#[gpui::test]
fn select_all_then_typing_replaces_everything(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("abc");
    cx.simulate_keystrokes("ctrl-a");
    cx.simulate_input("z");
    assert_eq!(text_of(&input, cx), "z");
}

#[gpui::test]
fn cut_copy_and_paste_round_trip_through_the_clipboard(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("hello");
    cx.simulate_keystrokes("ctrl-a ctrl-x");
    assert_eq!(text_of(&input, cx), "");
    cx.simulate_keystrokes("ctrl-v ctrl-v");
    assert_eq!(text_of(&input, cx), "hellohello");
}

#[gpui::test]
fn cut_without_a_selection_deletes_nothing(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("abc");
    cx.simulate_keystrokes("ctrl-x");
    assert_eq!(text_of(&input, cx), "abc");
}
