//! The Keybindings page.

use std::collections::BTreeMap;

use super::rows::row;
use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::page::Page;
use crate::section::Section;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;

pub(super) const OVERRIDES: &str = "keybindingUserOverrides";
pub(super) const PRESET: &str = "keybindingPreset";
const DEFAULT_PRESET: &str = "default";
const COMPACT_PRESET: &str = "compact";
const COMPACT: &[(&str, &str)] = &[("tabs.reopenClosed", "mod+shift+r")];
const PRESET_NOTE: &str = "Note: the standard IDE presets (VS Code, JetBrains, Sublime, Xcode, Atom, Emacs, Zed) are not yet provided - only Default and one alternate (Compact) ship real bindings.";

pub(super) fn preset_overrides(preset: &str) -> BTreeMap<String, String> {
    if preset != COMPACT_PRESET {
        return BTreeMap::new();
    }
    COMPACT
        .iter()
        .map(|&(command, chord)| (command.to_owned(), chord.to_owned()))
        .collect()
}

const NAVIGATION: &[(&str, &str, &str)] = &[
    ("navigation.toggleConsole", "Toggle console", "mod+`"),
    ("navigation.sidebarChats", "Sidebar: Chats", "mod+1"),
    ("navigation.sidebarFiles", "Sidebar: Files", "mod+2"),
    ("navigation.sidebarGit", "Sidebar: Git", "mod+3"),
];
const PANES: &[(&str, &str, &str)] = &[
    ("panes.navigateLeft", "Focus pane left", "mod+alt+arrowleft"),
    (
        "panes.navigateRight",
        "Focus pane right",
        "mod+alt+arrowright",
    ),
    ("panes.navigateUp", "Focus pane up", "mod+alt+arrowup"),
    ("panes.navigateDown", "Focus pane down", "mod+alt+arrowdown"),
];
const TABS: &[(&str, &str, &str)] = &[
    ("tabs.new", "New tab", "mod+t"),
    ("tabs.newTerminal", "New terminal tab", "mod+j"),
    ("agent.newChat", "New chat", "mod+n"),
    ("agent.newChatTerminal", "New chat (Terminal)", "mod+alt+n"),
    ("tabs.newFile", "New file", "mod+shift+n"),
    ("tabs.openFile", "Open file\u{2026}", "mod+o"),
    ("tabs.reopenClosed", "Reopen last closed tab", "mod+shift+t"),
    ("tabs.closeActive", "Close active tab", "mod+w"),
    (
        "browser.togglePick",
        "Pick element in browser tab",
        "mod+shift+e",
    ),
];
const CHATS: &[(&str, &str, &str)] = &[
    (
        "agent.toggleViewMode",
        "Toggle chat / terminal view",
        "mod+/",
    ),
    ("agent.cycleProvider", "Cycle chat provider", ""),
    ("agent.zoomIn", "Zoom chat in", "mod+="),
    ("agent.zoomOut", "Zoom chat out", "mod+-"),
    ("agent.zoomReset", "Reset chat zoom", "mod+0"),
];
const EDITOR: &[(&str, &str, &str)] = &[
    ("editor.save", "Save", "mod+s"),
    ("editor.saveAll", "Save all", "mod+shift+s"),
];

pub(super) fn page() -> Page {
    let groups = [
        ("Navigation", NAVIGATION),
        ("Panes", PANES),
        ("Tabs", TABS),
        ("Chats", CHATS),
        ("Editor", EDITOR),
    ];
    let page = Page::new("keybindings", "Keybindings", "keyboard").block(preset());
    let page = groups.iter().fold(page, |page, &(title, commands)| {
        page.block(group(title, commands))
    });
    page.block(ButtonSpec::new(
        "Reset all custom keybindings",
        Action::ResetOverrides(OVERRIDES.into()),
    ))
}

fn preset() -> Section {
    Section::new("Preset")
        .headerless()
        .describe("Choose a base keybinding set. Individual shortcuts below override it.")
        .row(row(
            (PRESET, "Active preset"),
            PRESET_NOTE,
            Control::select(
                &[
                    (DEFAULT_PRESET, "Default"),
                    (COMPACT_PRESET, "Compact (alternate)"),
                ],
                ControlWidth::Default,
            ),
            DEFAULT_PRESET,
        ))
}

fn group(
    title: &str,
    commands: &[(&str, &str, &str)],
) -> ShortcutGroup {
    commands.iter().fold(
        ShortcutGroup::new(title, OVERRIDES).preset_from(PRESET),
        |group, &(command, label, chord)| group.shortcut(in_presets(command, label, chord)),
    )
}

fn in_presets(
    command: &str,
    label: &str,
    chord: &str,
) -> Shortcut {
    let shortcut = Shortcut::new(command, label, chord);
    match COMPACT.iter().find(|(bound, _)| *bound == command) {
        Some((_, compact)) => shortcut.in_preset(COMPACT_PRESET, compact),
        None => shortcut,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn presets_bind_only_their_own_commands() {
        let cases = [("default", 0), ("compact", 1), ("emacs", 0)];
        for (preset, bound) in cases {
            assert_eq!(preset_overrides(preset).len(), bound, "{preset}");
        }
        assert_eq!(
            preset_overrides("compact")
                .get("tabs.reopenClosed")
                .map(String::as_str),
            Some("mod+shift+r")
        );
    }

    #[test]
    fn the_page_opens_with_the_preset_row_and_every_group_follows_it() {
        let page = page();
        let first = page.rows().next().map(|row| row.key().as_str().to_owned());
        assert_eq!(first.as_deref(), Some("keybindingPreset"));
        assert!(page.shortcut_groups().all(|group| group.preset.is_some()));
    }

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
}
