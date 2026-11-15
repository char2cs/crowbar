use startup_view::StartupCommand;

/// Something a view or a background thread asks the shell to do on the main thread.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum ShellRequest {
    Sidebar(sidebar::Command),
    Startup(StartupCommand),
    CloseSettings,
    Overlays { open: bool },
}
