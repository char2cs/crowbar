//! The Keybindings page.

use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::page::Page;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;

pub(super) const OVERRIDES: &str = "keybindingUserOverrides";

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
    let page = Page::new("keybindings", "Keybindings", "keyboard");
    let page = groups.iter().fold(page, |page, &(title, commands)| {
        page.block(group(title, commands))
    });
    page.block(ButtonSpec::new(
        "Reset all custom keybindings",
        Action::ResetOverrides(OVERRIDES.into()),
    ))
}

fn group(title: &str, commands: &[(&str, &str, &str)]) -> ShortcutGroup {
    commands.iter().fold(
        ShortcutGroup::new(title, OVERRIDES),
        |group, &(command, label, chord)| group.shortcut(Shortcut::new(command, label, chord)),
    )
}

#[cfg(test)]
#[path = "keybindings_tests.rs"]
mod tests;
