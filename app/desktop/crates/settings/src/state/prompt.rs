mod buttons;

use crate::schema::Confirm;
use crate::state::effect::Effect;

#[derive(Debug, Clone, PartialEq)]
pub struct Prompt {
    pub(crate) confirm: Confirm,
    pub(crate) then: Effect,
}

impl Prompt {
    #[must_use]
    pub fn confirm(&self) -> &Confirm {
        &self.confirm
    }

    pub fn title(&self) -> &str {
        &self.confirm.title
    }

    pub fn message(&self) -> &str {
        &self.confirm.message
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
            (prompt.title(), prompt.message(), prompt.confirm().action()),
            ("Kill?", "Ends the agent.", "Kill")
        );
    }
}
