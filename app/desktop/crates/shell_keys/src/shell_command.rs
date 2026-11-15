/// A command the shell serves itself, whichever side had keyboard focus.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum ShellCommand {
    /// Open an empty tab.
    NewTab,
    /// Start a chat.
    NewChat,
    /// Close the active tab.
    CloseTab,
    /// Open the tab that was closed last again.
    ReopenClosedTab,
    /// Open a terminal in the focused chat's workspace.
    NewTerminal,
    /// Open a browser tab.
    NewBrowser,
    /// Open a new, empty file in the focused chat's workspace.
    NewFile,
    /// Pick a file on this computer and open it.
    OpenFile,
    /// Open the review of the focused chat's branch.
    OpenReview,
    /// Show the daemon log, or hide it when it is showing.
    ToggleConsole,
    /// Move focus to the pane on the left.
    FocusPaneLeft,
    /// Move focus to the pane on the right.
    FocusPaneRight,
    /// Move focus to the pane above.
    FocusPaneUp,
    /// Move focus to the pane below.
    FocusPaneDown,
    /// Show or hide the sidebar.
    ToggleSidebar,
    /// Switch the sidebar to chats.
    SidebarChats,
    /// Switch the sidebar to files.
    SidebarFiles,
    /// Switch the sidebar to git changes.
    SidebarGit,
    /// Open the command palette.
    CommandPalette,
    /// Open the settings.
    OpenSettings,
    /// Show the log file in the system's default viewer.
    OpenLog,
    /// Zip the logs and the daemon's live dumps and show the file.
    ExportDiagnostics,
}
