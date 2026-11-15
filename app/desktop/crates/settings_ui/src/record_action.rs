//! A button on a record.

use crate::confirm::Confirm;

/// The action a record offers, such as Kill or Revoke.
#[derive(Debug, Clone, PartialEq)]
pub struct RecordAction {
    pub(crate) id: String,
    pub(crate) label: Option<String>,
    pub(crate) icon: String,
    pub(crate) tooltip: String,
    pub(crate) confirm: Option<Confirm>,
}

impl RecordAction {
    /// An action `id` drawn as the icon with file stem `icon`, explained by `tooltip`.
    pub fn new(
        id: impl Into<String>,
        icon: impl Into<String>,
        tooltip: impl Into<String>,
    ) -> Self {
        Self {
            id: id.into(),
            label: None,
            icon: icon.into(),
            tooltip: tooltip.into(),
            confirm: None,
        }
    }

    /// Shows `label` next to the icon.
    #[must_use]
    pub fn labelled(
        mut self,
        label: impl Into<String>,
    ) -> Self {
        self.label = Some(label.into());
        self
    }

    /// Asks `confirm` before the action runs.
    #[must_use]
    pub fn confirm(
        mut self,
        confirm: Confirm,
    ) -> Self {
        self.confirm = Some(confirm);
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_add_label_and_confirmation() {
        let action = RecordAction::new("kill", "skull", "Kill the agent")
            .labelled("Kill")
            .confirm(Confirm::new("Kill?", "Ends it.", "Kill"));
        assert_eq!(action.label.as_deref(), Some("Kill"));
        assert!(action.confirm.is_some());
        assert_eq!(
            (action.id.as_str(), action.icon.as_str()),
            ("kill", "skull")
        );
    }
}
