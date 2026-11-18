//! What the focused pane and the running agents say.

use crate::shell::switcher::focus::Focus;
use crate::shell::switcher::running_agent::RunningAgent;

/// What the focused pane and the running agents say.
pub trait PaneFocus {
    /// What the focused pane shows, which decides the available modes.
    fn focus(&self) -> Focus;
    /// The chats whose agent is running now, on every host.
    fn running_agents(&self) -> Vec<RunningAgent> {
        Vec::new()
    }
    /// Whether a thread can start in a terminal.
    fn can_start_terminal_thread(&self) -> bool;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixtures::DemoShell;

    #[test]
    fn the_demo_pane_can_start_a_terminal_thread_and_runs_no_agent() {
        let data = DemoShell::reference();
        assert!(data.can_start_terminal_thread());
        assert_eq!(data.running_agents().len(), 0);
        assert_eq!(data.focus(), Focus::Repository);
    }
}
