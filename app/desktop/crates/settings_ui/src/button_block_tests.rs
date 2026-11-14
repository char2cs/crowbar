use gpui::TestAppContext;

use crate::block::Block;
use crate::catalog::default_schema;
use crate::intent::Intent;
use crate::test_support::{open, run};

#[gpui::test]
fn the_add_host_button_runs_its_backend_action(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "hosts");
    let schema = default_schema().unwrap();
    let spec = schema
        .pages()
        .iter()
        .flat_map(|page| page.blocks.iter())
        .find_map(|block| match block {
            Block::Button(spec) if spec.label == "Add host" => Some(spec.clone()),
            _ => None,
        })
        .unwrap();
    run(&harness, Intent::Button(spec), cx);
    assert_eq!(harness.backend.invoked(), ["hosts.add"]);
}

#[gpui::test]
fn reset_all_clears_every_custom_chord_without_asking(cx: &mut TestAppContext) {
    use crate::catalog::KEYBINDING_OVERRIDES;
    use crate::key::Key;
    use crate::modifier::Modifier;
    use crate::pressed::Pressed;
    use crate::settings_backend::SettingsBackend;
    use crate::value::Value;

    let (harness, cx) = open(cx, "keybindings");
    run(
        &harness,
        Intent::Capture("navigation.toggleConsole".to_owned()),
        cx,
    );
    run(
        &harness,
        Intent::Key(Pressed::new("k", &[Modifier::Control])),
        cx,
    );
    let schema = default_schema().unwrap();
    let spec = schema
        .pages()
        .iter()
        .flat_map(|page| page.blocks.iter())
        .find_map(|block| match block {
            Block::Button(spec) if spec.label.starts_with("Reset") => Some(spec.clone()),
            _ => None,
        })
        .unwrap();
    run(&harness, Intent::Button(spec), cx);
    assert!(
        matches!(harness.backend.get(&Key::new(KEYBINDING_OVERRIDES)), Some(Value::Map(map)) if map.is_empty())
    );
}
