//! A confirmation waiting for the user.

use crate::confirm::Confirm;
use crate::effect::Effect;

/// A question asked before `then` happens.
#[derive(Debug, Clone, PartialEq)]
pub struct Prompt {
    pub(crate) confirm: Confirm,
    pub(crate) then: Effect,
}

impl Prompt {
    /// The title of the question.
    pub fn title(&self) -> &str {
        &self.confirm.title
    }

    /// The explanation.
    pub fn message(&self) -> &str {
        &self.confirm.message
    }

    /// The label of the confirming button.
    pub fn confirm_label(&self) -> &str {
        &self.confirm.action
    }
}

#[cfg(test)]
#[path = "prompt_tests.rs"]
mod tests;
