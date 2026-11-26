//! What a button does.

use crate::values::Key;

/// The effect of pressing a button in the schema.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Action {
    /// Drops every custom keybinding stored under this key.
    ResetOverrides(Key),
    /// Asks the backend to run the named action.
    Backend(String),
}

impl Action {
    pub(crate) fn backend(id: impl Into<String>) -> Self {
        Self::Backend(id.into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn backend_actions_carry_their_id() {
        assert_eq!(
            Action::backend("hosts.add"),
            Action::Backend("hosts.add".to_owned())
        );
    }
}
