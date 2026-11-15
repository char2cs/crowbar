use std::cell::RefCell;

use gpui::App;

use crate::startup_actions::StartupActions;
use crate::startup_command::StartupCommand;

/// An action sink that remembers what it was asked.
#[derive(Debug, Default)]
pub(crate) struct RecordingActions {
    performed: RefCell<Vec<StartupCommand>>,
}

impl RecordingActions {
    /// Every command performed so far, oldest first.
    pub(crate) fn performed(&self) -> Vec<StartupCommand> {
        self.performed.borrow().clone()
    }
}

impl StartupActions for RecordingActions {
    fn perform(
        &self,
        command: StartupCommand,
        _cx: &mut App,
    ) {
        self.performed.borrow_mut().push(command);
    }
}
