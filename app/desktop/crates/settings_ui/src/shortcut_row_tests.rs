use gpui::TestAppContext;

use crate::catalog::KEYBINDING_OVERRIDES;
use crate::intent::Intent;
use crate::key::Key;
use crate::modifier::Modifier;
use crate::pressed::Pressed;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{inspect, open, run};
use crate::value::Value;

const COMMAND: &str = "navigation.toggleConsole";

fn overrides(harness: &crate::test_support::Harness) -> Option<Value> {
    harness.backend.get(&Key::new(KEYBINDING_OVERRIDES))
}

#[gpui::test]
fn a_modified_key_is_stored_and_clearing_it_restores_the_default(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "keybindings");
    run(&harness, Intent::Capture(COMMAND.to_owned()), cx);
    assert_eq!(
        inspect(&harness, cx, |modal| modal
            .state
            .capturing()
            .map(str::to_owned)),
        Some(COMMAND.to_owned())
    );
    run(
        &harness,
        Intent::Key(Pressed::new("k", &[Modifier::Control])),
        cx,
    );
    assert!(matches!(overrides(&harness), Some(Value::Map(map)) if map.len() == 1));
    run(&harness, Intent::ClearChord(COMMAND.to_owned()), cx);
    assert!(matches!(overrides(&harness), Some(Value::Map(map)) if map.is_empty()));
}

#[gpui::test]
fn a_bare_printable_key_is_refused_and_stores_nothing(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "keybindings");
    run(&harness, Intent::Capture(COMMAND.to_owned()), cx);
    run(&harness, Intent::Key(Pressed::new("a", &[])), cx);
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .capture_refusal()
        .is_some()));
    assert_eq!(overrides(&harness), None);
    run(&harness, Intent::Key(Pressed::new("escape", &[])), cx);
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .capturing()
        .is_none()));
}
