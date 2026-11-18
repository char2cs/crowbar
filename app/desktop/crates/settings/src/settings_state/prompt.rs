//! A confirmation waiting for the user.

mod buttons;

use crate::schema::Confirm;
use crate::settings_state::effect::Effect;

/// A question asked before `then` happens.
#[derive(Debug, Clone, PartialEq)]
pub struct Prompt {
    /// The wording of the question.
    pub confirm: Confirm,
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
mod tests {
    use super::*;

    #[test]
    fn exposes_the_wording() {
        let prompt = Prompt {
            confirm: Confirm::new("Kill?", "Ends the agent.", "Kill"),
            then: Effect::Invoke("kill".to_owned()),
        };
        assert_eq!(
            (prompt.title(), prompt.message(), prompt.confirm_label()),
            ("Kill?", "Ends the agent.", "Kill")
        );
    }
}
