use crate::command_entry::CommandEntry;
use crate::shell_command::ShellCommand;

const fn entry(
    command: ShellCommand,
    id: &'static str,
    label: &'static str,
    chord: &'static str,
) -> CommandEntry {
    CommandEntry {
        command,
        id,
        label,
        chord,
    }
}

const NEW_TAB: CommandEntry = entry(ShellCommand::NewTab, "tabs.new", "New tab", "mod+t");

// SPEC: app/panes/src/features/keymaps/registry.ts is the table these ids and chords follow.
const TABLE: &[CommandEntry] = &[
    NEW_TAB,
    entry(ShellCommand::NewChat, "agent.newChat", "New chat", "mod+n"),
    entry(
        ShellCommand::NewChatTerminal,
        "agent.newChatTerminal",
        "New chat (Terminal)",
        "mod+alt+n",
    ),
    entry(
        ShellCommand::CloseTab,
        "tabs.closeActive",
        "Close active tab",
        "mod+w",
    ),
    entry(
        ShellCommand::ReopenClosedTab,
        "tabs.reopenClosed",
        "Reopen last closed tab",
        "mod+shift+t",
    ),
    entry(
        ShellCommand::NewTerminal,
        "tabs.newTerminal",
        "New terminal tab",
        "mod+j",
    ),
    entry(
        ShellCommand::NewBrowser,
        "tabs.newBrowser",
        "New browser tab",
        "",
    ),
    entry(
        ShellCommand::NewFile,
        "tabs.newFile",
        "New file",
        "mod+shift+n",
    ),
    entry(
        ShellCommand::OpenFile,
        "tabs.openFile",
        "Open file",
        "mod+o",
    ),
    entry(
        ShellCommand::OpenReview,
        "git.openReview",
        "Review branch",
        "",
    ),
    entry(
        ShellCommand::ToggleConsole,
        "navigation.toggleConsole",
        "Toggle console",
        "mod+`",
    ),
    entry(
        ShellCommand::FocusPaneLeft,
        "panes.navigateLeft",
        "Focus pane left",
        "mod+alt+arrowleft",
    ),
    entry(
        ShellCommand::FocusPaneRight,
        "panes.navigateRight",
        "Focus pane right",
        "mod+alt+arrowright",
    ),
    entry(
        ShellCommand::FocusPaneUp,
        "panes.navigateUp",
        "Focus pane up",
        "mod+alt+arrowup",
    ),
    entry(
        ShellCommand::FocusPaneDown,
        "panes.navigateDown",
        "Focus pane down",
        "mod+alt+arrowdown",
    ),
    entry(
        ShellCommand::ToggleSidebar,
        "navigation.toggleSidebar",
        "Toggle sidebar",
        "mod+b",
    ),
    entry(
        ShellCommand::SidebarChats,
        "navigation.sidebarChats",
        "Sidebar: Chats",
        "mod+1",
    ),
    entry(
        ShellCommand::SidebarFiles,
        "navigation.sidebarFiles",
        "Sidebar: Files",
        "mod+2",
    ),
    entry(
        ShellCommand::SidebarGit,
        "navigation.sidebarGit",
        "Sidebar: Git",
        "mod+3",
    ),
    entry(
        ShellCommand::CommandPalette,
        "navigation.commandPalette",
        "Command palette",
        "mod+shift+p",
    ),
    entry(
        ShellCommand::OpenSettings,
        "app.openSettings",
        "Open settings",
        "mod+,",
    ),
    entry(ShellCommand::OpenLog, "app.openLog", "Open log file", ""),
    entry(
        ShellCommand::ExportDiagnostics,
        "app.exportDiagnostics",
        "Export diagnostics",
        "",
    ),
];

impl ShellCommand {
    /// Every command, in menu order.
    pub fn all() -> impl Iterator<Item = Self> {
        TABLE.iter().map(|entry| entry.command)
    }

    /// The command with id `id`, `None` for one a pane or extension serves.
    #[must_use]
    pub fn parse(id: &str) -> Option<Self> {
        TABLE
            .iter()
            .find(|entry| entry.id == id)
            .map(|entry| entry.command)
    }

    /// The command id used in settings and in `command.run` frames.
    #[must_use]
    pub fn id(self) -> &'static str {
        self.entry().id
    }

    /// The name shown in menus and the palette.
    #[must_use]
    pub fn label(self) -> &'static str {
        self.entry().label
    }

    /// The chord the command has until the user rebinds it.
    #[must_use]
    pub fn default_chord(self) -> &'static str {
        self.entry().chord
    }

    fn entry(self) -> &'static CommandEntry {
        TABLE
            .iter()
            .find(|entry| entry.command == self)
            .unwrap_or(&NEW_TAB)
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashSet;

    use super::*;

    #[test]
    fn every_command_round_trips_through_its_id() {
        for command in ShellCommand::all() {
            assert_eq!(ShellCommand::parse(command.id()), Some(command));
        }
    }

    #[test]
    fn ids_and_default_chords_are_unique() {
        let ids: HashSet<_> = ShellCommand::all().map(ShellCommand::id).collect();
        let bound: Vec<_> = ShellCommand::all()
            .map(ShellCommand::default_chord)
            .filter(|chord| !chord.is_empty())
            .collect();
        let chords: HashSet<_> = bound.iter().collect();
        assert_eq!(ids.len(), ShellCommand::all().count(), "ids collide");
        assert_eq!(chords.len(), bound.len(), "default chords collide");
    }

    #[test]
    fn only_shell_commands_parse_and_the_documented_chords_are_the_defaults() {
        const CHORD_CASES: &[(ShellCommand, &str)] = &[
            (ShellCommand::NewTab, "mod+t"),
            (ShellCommand::CloseTab, "mod+w"),
            (ShellCommand::NewChat, "mod+n"),
        ];
        const NOT_SHELL: &[&str] = &["editor.save", "agent.zoomIn", "ext.git.openReview", ""];
        for id in NOT_SHELL {
            assert_eq!(ShellCommand::parse(id), None, "{id:?}");
        }
        for (command, chord) in CHORD_CASES {
            assert_eq!(command.default_chord(), *chord, "{command:?}");
        }
    }
}
