use ui::{ButtonVariant, IconName};

/// What the user can ask of the shell from the startup view.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum StartupCommand {
    /// Start the daemon again.
    Retry,
    /// Show the daemon log.
    OpenLog,
    /// Quit Crowbar.
    Quit,
}

impl StartupCommand {
    /// The button label.
    pub fn label(self) -> &'static str {
        match self {
            Self::Retry => "Try again",
            Self::OpenLog => "Open log",
            Self::Quit => "Quit Crowbar",
        }
    }

    pub(crate) fn icon(self) -> IconName {
        match self {
            Self::Retry => IconName::ArrowClockwise,
            Self::OpenLog => IconName::TerminalWindow,
            Self::Quit => IconName::X,
        }
    }

    pub(crate) fn variant(self, primary: Self) -> ButtonVariant {
        if self == primary {
            ButtonVariant::Primary
        } else {
            ButtonVariant::Outline
        }
    }

    pub(crate) fn id(self) -> &'static str {
        match self {
            Self::Retry => "startup-retry",
            Self::OpenLog => "startup-open-log",
            Self::Quit => "startup-quit",
        }
    }
}

#[cfg(test)]
#[path = "startup_command_tests.rs"]
mod tests;
