use gpui::TestAppContext;

use crate::key::Key;
use crate::test_support::{inspect, open};
use crate::value::Value;

#[gpui::test]
fn inputs_follow_the_state_when_another_window_changes_a_value(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "browser");
    let key = Key::new("browserHomepage");
    let input = inspect(&harness, cx, |modal| modal.fields.get(&key).cloned()).unwrap();
    harness
        .backend
        .publish(&key, Value::Text("https://a.test".to_owned()));
    cx.run_until_parked();
    assert_eq!(
        input.read_with(cx, |input, _| input.text().to_owned()),
        "https://a.test"
    );
}

#[gpui::test]
fn only_rows_on_a_visited_page_get_an_input(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    assert!(inspect(&harness, cx, |modal| modal
        .fields
        .get(&Key::new("browserHomepage"))
        .is_none()));
    assert!(inspect(&harness, cx, |modal| modal
        .fields
        .get(&Key::new("uiFontSize"))
        .is_some()));
}
