use gpui::TestAppContext;

use crate::intent::Intent;
use crate::key::Key;
use crate::nudge::Nudge;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{inspect, open, row, run};
use crate::value::Value;

#[gpui::test]
fn the_buttons_step_by_the_range_step_and_stop_at_its_ends(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let target = row("uiFontSize");
    run(&harness, Intent::Nudge(target.clone(), Nudge::Up), cx);
    assert_eq!(
        harness.backend.get(&Key::new("uiFontSize")),
        Some(Value::Number(15.5))
    );
    harness
        .backend
        .publish(&Key::new("uiFontSize"), Value::Number(24.0));
    cx.run_until_parked();
    run(&harness, Intent::Nudge(target.clone(), Nudge::Up), cx);
    assert_eq!(
        harness.backend.get(&Key::new("uiFontSize")),
        Some(Value::Number(24.0))
    );
    assert_eq!(
        inspect(&harness, cx, |modal| modal.state.field_text(&target)),
        "24"
    );
}
