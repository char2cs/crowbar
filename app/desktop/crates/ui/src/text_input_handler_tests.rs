use gpui::{EntityInputHandler as _, TestAppContext};

use crate::test_support::{field, text_of};

#[gpui::test]
fn composition_is_marked_and_replaced_by_the_committed_text(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    let marked = input.update_in(cx, |input, window, cx| {
        input.replace_and_mark_text_in_range(None, "ni", None, window, cx);
        input.marked_text_range(window, cx)
    });
    assert_eq!(marked, Some(0..2));
    assert_eq!(text_of(&input, cx), "ni");
    input.update_in(cx, |input, window, cx| {
        input.replace_text_in_range(None, "你", window, cx);
    });
    assert_eq!(text_of(&input, cx), "你");
}

#[gpui::test]
fn ranges_are_reported_in_utf16_units(cx: &mut TestAppContext) {
    let (input, cx) = field(cx);
    cx.simulate_input("a😀b");
    let selection = input.update_in(cx, |input, window, cx| {
        input.selected_text_range(false, window, cx)
    });
    assert_eq!(selection.map(|selection| selection.range), Some(4..4));
    let slice = input.update_in(cx, |input, window, cx| {
        input.text_for_range(1..3, &mut None, window, cx)
    });
    assert_eq!(slice.as_deref(), Some("😀"));
}
