use startup_view::StartupCommand;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum ShellRequest {
    Sidebar(sidebar::Command),
    Startup(StartupCommand),
    CloseSettings,
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
    Browse {
        pane: protocol::PaneId,
        event: browser_chrome::BrowserBarEvent,
    },
}
