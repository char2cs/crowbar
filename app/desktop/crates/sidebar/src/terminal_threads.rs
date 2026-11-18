//! Whether threads can start in a terminal.

use crate::sidebar_data::SidebarData;

/// Whether the sidebar offers to start a thread in a terminal; `Unavailable` is the default.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum TerminalThreads {
    /// The data source cannot start terminal threads; the row menu leaves that entry out.
    #[default]
    Unavailable,
    /// The data source can start terminal threads.
    Available,
}

impl TerminalThreads {
    /// Whether the data source can start a thread in a terminal.
    pub fn of(data: &dyn SidebarData) -> Self {
        if data.can_start_terminal_thread() {
            return Self::Available;
        }
        Self::Unavailable
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_provider_that_can_start_terminal_threads_is_available() {
        let data = crate::demo_data::DemoData::reference();
        assert_eq!(TerminalThreads::of(&data), TerminalThreads::Available);
    }

    #[test]
    fn unavailable_is_the_default() {
        assert_eq!(TerminalThreads::default(), TerminalThreads::Unavailable);
    }
}
