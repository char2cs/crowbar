use gpui::TestAppContext;

use crate::catalog::KEYBINDING_OVERRIDES;
use crate::intent::Intent;
use crate::key::Key;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{Harness, inspect, open};
use crate::value::Value;

fn focus_body(harness: &Harness, cx: &mut gpui::VisualTestContext) {
    harness
        .modal
        .update_in(cx, |modal, window, _| modal.focus.focus(window));
    cx.run_until_parked();
}

fn page_id(harness: &Harness, cx: &mut gpui::VisualTestContext) -> String {
    inspect(harness, cx, |modal| {
        modal
            .state
            .page()
            .map_or_else(String::new, |page| page.id.to_string())
    })
}

#[gpui::test]
fn arrow_keys_walk_the_rail_and_escape_closes(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    focus_body(&harness, cx);
    cx.simulate_keystrokes("down");
    assert_eq!(page_id(&harness, cx), "editor");
    cx.simulate_keystrokes("end");
    assert_eq!(page_id(&harness, cx), "hosts");
    cx.simulate_keystrokes("up");
    assert_eq!(page_id(&harness, cx), "keybindings");
    cx.simulate_keystrokes("escape");
    assert_eq!(harness.closed.get(), 1);
}

#[gpui::test]
fn while_capturing_a_chord_keys_go_to_the_capture(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "keybindings");
    focus_body(&harness, cx);
    crate::test_support::run(
        &harness,
        Intent::Capture("navigation.toggleConsole".to_owned()),
        cx,
    );
    cx.simulate_keystrokes("a");
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .capture_refusal()
        .is_some()));
    cx.simulate_keystrokes("ctrl-k");
    let stored = harness.backend.get(&Key::new(KEYBINDING_OVERRIDES));
    let Some(Value::Map(map)) = stored else {
        panic!("no overrides stored: {stored:?}");
    };
    assert_eq!(
        map.get("navigation.toggleConsole"),
        Some(&"mod+k".to_owned())
    );
    assert_eq!(harness.closed.get(), 0);
}
