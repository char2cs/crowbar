use crate::command_entry::CommandEntry;
use crate::shell_command::ShellCommand;

const fn entry(
    command: ShellCommand,
    id: &'static str,
    chord: &'static str,
) -> CommandEntry {
    CommandEntry { command, id, chord }
}

const NEW_TAB: CommandEntry = entry(ShellCommand::NewTab, "tabs.new", "mod+t");

// SPEC: app/panes/src/features/keymaps/registry.ts is the table these ids and chords follow.
const TABLE: &[CommandEntry] = &[
    NEW_TAB,
    entry(ShellCommand::NewChat, "agent.newChat", "mod+n"),
    entry(
        ShellCommand::NewChatTerminal,
        "agent.newChatTerminal",
        "mod+alt+n",
    ),
    entry(ShellCommand::CloseTab, "tabs.closeActive", "mod+w"),
    entry(
        ShellCommand::ReopenClosedTab,
        "tabs.reopenClosed",
        "mod+shift+t",
    ),
    entry(ShellCommand::NewTerminal, "tabs.newTerminal", "mod+j"),
    entry(ShellCommand::NewBrowser, "tabs.newBrowser", ""),
    entry(ShellCommand::NewFile, "tabs.newFile", "mod+shift+n"),
    entry(ShellCommand::OpenFile, "tabs.openFile", "mod+o"),
    entry(ShellCommand::OpenReview, "git.openReview", ""),
    entry(
        ShellCommand::ToggleConsole,
        "navigation.toggleConsole",
        "mod+`",
    ),
    entry(
        ShellCommand::FocusPaneLeft,
        "panes.navigateLeft",
        "mod+alt+arrowleft",
    ),
    entry(
        ShellCommand::FocusPaneRight,
        "panes.navigateRight",
        "mod+alt+arrowright",
    ),
    entry(
        ShellCommand::FocusPaneUp,
        "panes.navigateUp",
        "mod+alt+arrowup",
    ),
    entry(
        ShellCommand::FocusPaneDown,
        "panes.navigateDown",
        "mod+alt+arrowdown",
    ),
    entry(
        ShellCommand::ToggleSidebar,
        "navigation.toggleSidebar",
        "mod+b",
    ),
    entry(
        ShellCommand::SidebarChats,
        "navigation.sidebarChats",
        "mod+1",
    ),
    entry(
        ShellCommand::SidebarFiles,
        "navigation.sidebarFiles",
        "mod+2",
    ),
    entry(ShellCommand::SidebarGit, "navigation.sidebarGit", "mod+3"),
    entry(ShellCommand::OpenLog, "app.openLog", ""),
    entry(ShellCommand::ExportDiagnostics, "app.exportDiagnostics", ""),
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
