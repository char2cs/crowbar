//! Whether threads can start in a terminal.

/// The provider's capability.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum TerminalThreads {
    /// The provider cannot.
    #[default]
    Unavailable,
    /// The provider can.
    Available,
}

impl From<bool> for TerminalThreads {
    fn from(available: bool) -> Self {
        if available {
            Self::Available
        } else {
            Self::Unavailable
        }
    }
}

#[cfg(test)]
#[path = "terminal_threads_tests.rs"]
mod tests;
