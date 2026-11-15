//! Whether threads can start in a terminal.

use crate::sidebar_data::SidebarData;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum TerminalThreads {
    #[default]
    Unavailable,
    Available,
}

impl TerminalThreads {
    pub(crate) fn of(data: &dyn SidebarData) -> Self {
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
