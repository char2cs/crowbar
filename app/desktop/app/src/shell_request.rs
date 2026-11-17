use startup_view::StartupCommand;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum ShellRequest {
    Sidebar(sidebar::Command),
    Startup(StartupCommand),
    CloseSettings,
    /// A pane reported its unsaved work again.
    ResumeClosing,
    /// A `mod+shift+<digit>` press selected the project in this slot (1 to 9).
    SelectProject(u8),
    /// The user answered the unsaved changes question.
    Closing(crate::close_choice::CloseChoice),
    DataChanged,
    Outcome(shell_data::Outcome),
    Overlays {
        open: bool,
    },
    Command(shell_keys::ShellCommand),
    /// The stored keybinding overrides changed.
    KeymapChanged,
    /// A setting the panes follow changed.
    PaneSettingsChanged,
    /// The stored theme, theme mode or UI font size changed.
    AppearanceChanged,
    NewTabChoice {
        pane: protocol::PaneId,
        action: crate::new_tab_action::NewTabAction,
    },
    /// The daemon asked for something of a browser tab.
    Agent(browser_agent::AgentCommand),
    Browse {
        pane: protocol::PaneId,
        event: browser_chrome::BrowserBarEvent,
    },
}
