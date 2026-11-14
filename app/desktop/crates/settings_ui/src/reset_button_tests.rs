use gpui::TestAppContext;

use crate::intent::Intent;
use crate::key::Key;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{inspect, open, row, run};
use crate::value::Value;

#[gpui::test]
fn reset_writes_the_default_back_and_the_row_stops_being_resettable(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let target = row("sidebarFollowsFocus");
    assert!(!inspect(&harness, cx, |modal| modal
        .state
        .resettable(&target)));
    run(&harness, Intent::Toggle(target.clone()), cx);
    run(&harness, Intent::Reset(target.clone()), cx);
    assert_eq!(
        harness.backend.get(&Key::new("sidebarFollowsFocus")),
        Some(Value::Bool(false))
    );
    assert!(!inspect(&harness, cx, |modal| modal
        .state
        .resettable(&target)));
}
