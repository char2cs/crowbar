pub(super) mod focus;
pub(super) mod running_agent;

use crate::shell::switcher::pane_focus::focus::Focus;
use crate::shell::switcher::pane_focus::running_agent::RunningAgent;

pub trait PaneFocus {
    fn focus(&self) -> Focus;
    fn running_agents(&self) -> Vec<RunningAgent> {
        Vec::new()
    }
    fn can_start_terminal_thread(&self) -> bool;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::DemoShell;

    #[test]
    fn the_demo_pane_can_start_a_terminal_thread_and_runs_no_agent() {
        let data = DemoShell::reference();
        assert!(data.can_start_terminal_thread());
        assert_eq!(data.running_agents().len(), 0);
        assert_eq!(data.focus(), Focus::Repository);
    }
}
