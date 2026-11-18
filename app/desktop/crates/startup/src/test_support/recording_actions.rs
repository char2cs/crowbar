use std::cell::RefCell;

use gpui::App;

use crate::actions::Actions;
use crate::command::Command;

/// An action sink that remembers what it was asked.
#[derive(Debug, Default)]
pub(crate) struct RecordingActions {
    performed: RefCell<Vec<Command>>,
}

impl RecordingActions {
    /// Every command performed so far, oldest first.
    pub(crate) fn performed(&self) -> Vec<Command> {
        self.performed.borrow().clone()
    }
}

impl Actions for RecordingActions {
    fn perform(
        &self,
        command: Command,
        _cx: &mut App,
    ) {
        self.performed.borrow_mut().push(command);
    }
}
