use gpui::TestAppContext;

use crate::intent::Intent;
use crate::key::Key;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{inspect, open, row, run};
use crate::value::Value;

#[gpui::test]
fn choosing_an_option_writes_its_value(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let target = row("themeMode");
    assert_eq!(
        inspect(&harness, cx, |modal| modal.state.selected_index(&target)),
        Some(0)
    );
    run(&harness, Intent::Choose(target.clone(), 2), cx);
    assert_eq!(
        harness.backend.get(&Key::new("themeMode")),
        Some(Value::Text("dark".to_owned()))
    );
    assert_eq!(
        inspect(&harness, cx, |modal| modal.state.selected_index(&target)),
        Some(2)
    );
}
