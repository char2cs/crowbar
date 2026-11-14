use super::*;

#[test]
fn shortcuts_keep_their_order() {
    let group = ShortcutGroup::new("Tabs", "overrides")
        .shortcut(Shortcut::new("a", "A", "mod+a"))
        .shortcut(Shortcut::new("b", "B", ""));
    let commands: Vec<&str> = group.shortcuts.iter().map(Shortcut::command).collect();
    assert_eq!(commands, ["a", "b"]);
    assert_eq!(group.overrides.as_str(), "overrides");
}
