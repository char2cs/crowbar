//! A confirmation waiting for the user.

mod buttons;

use crate::schema::Confirm;
use crate::state::effect::Effect;

/// A question asked before `then` happens.
#[derive(Debug, Clone, PartialEq)]
pub struct Prompt {
    pub(crate) confirm: Confirm,
    pub(crate) then: Effect,
}

impl Prompt {
    /// The wording of the question.
    #[must_use]
    pub fn confirm(&self) -> &Confirm {
        &self.confirm
    }

    /// The title of the question.
    pub fn title(&self) -> &str {
        &self.confirm.title
    }

    /// The explanation.
    pub fn message(&self) -> &str {
        &self.confirm.message
    }
}
