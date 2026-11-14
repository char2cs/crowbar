use super::*;

#[test]
fn the_registry_has_every_command_once_in_category_order() {
    let page = page();
    let groups: Vec<&ShortcutGroup> = page.shortcut_groups().collect();
    let titles: Vec<&str> = groups.iter().map(|group| group.title.as_str()).collect();
    assert_eq!(titles, ["Navigation", "Panes", "Tabs", "Chats", "Editor"]);
    let mut commands: Vec<&str> = groups
        .iter()
        .flat_map(|group| group.shortcuts.iter().map(Shortcut::command))
        .collect();
    assert_eq!(commands.len(), 24);
    commands.sort_unstable();
    commands.dedup();
    assert_eq!(commands.len(), 24, "no command repeats");
}

#[test]
fn every_default_chord_parses_and_only_one_command_ships_unbound() {
    let page = page();
    let unbound: Vec<&str> = page
        .shortcut_groups()
        .flat_map(|group| group.shortcuts.iter())
        .filter(|shortcut| shortcut.default.is_none())
        .map(Shortcut::command)
        .collect();
    assert_eq!(unbound, ["agent.cycleProvider"]);
}
