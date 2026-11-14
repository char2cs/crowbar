use gpui::TestAppContext;

use crate::key::Key;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{inspect, open};
use crate::value::Value;

fn homepage(
    cx: &mut TestAppContext,
) -> (
    crate::test_support::Harness,
    &mut gpui::VisualTestContext,
    gpui::Entity<ui::TextInput>,
) {
    let (harness, cx) = open(cx, "browser");
    cx.update(|window, _| window.activate_window());
    let input = inspect(&harness, cx, |modal| {
        modal.fields.get(&Key::new("browserHomepage")).cloned()
    })
    .unwrap();
    cx.update(|window, cx| input.read(cx).focus(window));
    (harness, cx, input)
}

#[gpui::test]
fn enter_commits_the_typed_text(cx: &mut TestAppContext) {
    let (harness, cx, _) = homepage(cx);
    cx.simulate_input("https://x.test");
    assert_eq!(harness.backend.get(&Key::new("browserHomepage")), None);
    cx.simulate_keystrokes("enter");
    assert_eq!(
        harness.backend.get(&Key::new("browserHomepage")),
        Some(Value::Text("https://x.test".to_owned()))
    );
}

#[gpui::test]
fn escape_throws_the_draft_away(cx: &mut TestAppContext) {
    let (harness, cx, input) = homepage(cx);
    cx.simulate_input("https://x.test");
    cx.simulate_keystrokes("escape");
    assert_eq!(harness.backend.get(&Key::new("browserHomepage")), None);
    assert_eq!(input.read_with(cx, |input, _| input.text().to_owned()), "");
}

#[gpui::test]
fn leaving_the_field_commits_it(cx: &mut TestAppContext) {
    let (harness, cx, _) = homepage(cx);
    cx.simulate_input("https://y.test");
    let before = inspect(&harness, cx, |modal| {
        modal
            .state
            .is_dirty(&crate::test_support::row("browserHomepage"))
    });
    assert!(before, "typing is a draft");
    harness
        .modal
        .update_in(cx, |modal, window, _| modal.focus.focus(window));
    cx.run_until_parked();
    cx.run_until_parked();
    assert_eq!(
        harness.backend.get(&Key::new("browserHomepage")),
        Some(Value::Text("https://y.test".to_owned()))
    );
}
