mod command_entry;
mod table;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum ShellCommand {
    NewTab,
    NewChat,
    NewChatTerminal,
    CloseTab,
    ReopenClosedTab,
    NewTerminal,
    NewBrowser,
    NewFile,
    OpenFile,
    OpenReview,
    ToggleConsole,
    FocusPaneLeft,
    FocusPaneRight,
    FocusPaneUp,
    FocusPaneDown,
    ToggleEditor,
    ToggleSidebar,
    SidebarChats,
    SidebarFiles,
    SidebarGit,
    OpenLog,
    ExportDiagnostics,
    ZoomChatIn,
    ZoomChatOut,
    ZoomChatReset,
    SaveAll,
}
