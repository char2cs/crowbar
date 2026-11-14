use super::*;
use crate::page::Page;
use crate::value::Value;
use crate::values::Values;

fn schema() -> Schema {
    let group = ShortcutGroup::new("Tabs", "overrides")
        .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t"))
        .shortcut(Shortcut::new(
            "tabs.newTerminal",
            "New terminal tab",
            "mod+j",
        ))
        .shortcut(Shortcut::new(
            "agent.cycleProvider",
            "Cycle chat provider",
            "",
        ));
    Schema::new(vec![
        Page::new("keybindings", "Keybindings", "keyboard").block(group),
    ])
    .unwrap()
}

fn document(overrides: &[(&str, &str)]) -> Document {
    let map = overrides
        .iter()
        .map(|&(k, v)| (k.to_owned(), v.to_owned()))
        .collect();
    Document::new(
        Values::default().with("overrides", Value::Map(map)),
        Values::default(),
    )
}

fn first_group(schema: &Schema) -> &ShortcutGroup {
    schema
        .pages()
        .first()
        .and_then(|page| page.shortcut_groups().next())
        .unwrap()
}

fn chord_of(schema: &Schema, document: &Document, command: &str) -> Option<String> {
    let group = first_group(schema);
    let shortcut = group
        .shortcuts
        .iter()
        .find(|s| s.command == command)
        .unwrap();
    Keymap::new(schema, document)
        .chord(group, shortcut)
        .map(|c| c.as_str().to_owned())
}

#[test]
fn overrides_win_over_defaults_and_empty_means_unbound() {
    let schema = schema();
    let plain = document(&[]);
    assert_eq!(
        chord_of(&schema, &plain, "tabs.new").as_deref(),
        Some("mod+t")
    );
    assert_eq!(chord_of(&schema, &plain, "agent.cycleProvider"), None);
    let custom = document(&[("tabs.new", "Mod+Shift+N"), ("tabs.newTerminal", "")]);
    assert_eq!(
        chord_of(&schema, &custom, "tabs.new").as_deref(),
        Some("mod+shift+n")
    );
    assert_eq!(chord_of(&schema, &custom, "tabs.newTerminal"), None);
}

#[test]
fn sources_and_custom_detection() {
    let schema = schema();
    let group = first_group(&schema);
    let custom = document(&[("tabs.new", "mod+n")]);
    let keymap = Keymap::new(&schema, &custom);
    let sources: Vec<ChordSource> = group
        .shortcuts
        .iter()
        .map(|s| keymap.source(group, s))
        .collect();
    assert_eq!(
        sources,
        [
            ChordSource::Custom,
            ChordSource::Default,
            ChordSource::Default
        ]
    );
    assert!(keymap.has_custom());
    assert!(!Keymap::new(&schema, &document(&[])).has_custom());
}

#[test]
fn a_chord_shared_by_two_commands_is_a_conflict_for_both() {
    let schema = schema();
    let group = first_group(&schema);
    let clash = document(&[("tabs.newTerminal", "mod+t")]);
    let keymap = Keymap::new(&schema, &clash);
    let found: Vec<Vec<String>> = group
        .shortcuts
        .iter()
        .map(|s| keymap.conflicts(group, s))
        .collect();
    assert_eq!(found[0], ["New terminal tab"]);
    assert_eq!(found[1], ["New tab"]);
    assert!(
        found[2].is_empty(),
        "an unbound command conflicts with nothing"
    );
}
