//! A button in a row or between blocks.

mod action;
mod confirm;

pub use action::Action;
pub use confirm::Confirm;

use crate::schema::condition::Condition;

/// A labelled button that runs an [`Action`], optionally after a confirmation.
#[derive(Debug, Clone, PartialEq)]
pub struct ButtonSpec {
    pub(crate) label: String,
    pub(crate) icon: Option<String>,
    pub(crate) action: Action,
    pub(crate) confirm: Option<Confirm>,
    pub(crate) enabled: Option<Condition>,
    pub(crate) disabled_note: Option<String>,
    pub(crate) note: Option<String>,
}

impl ButtonSpec {
    /// The text on the button.
    #[must_use]
    pub fn label(&self) -> &str {
        &self.label
    }

    /// File stem of the Phosphor icon shown before the label, if any.
    #[must_use]
    pub fn icon_stem(&self) -> Option<&str> {
        self.icon.as_deref()
    }

    /// What the button does when pressed.
    #[must_use]
    pub fn action(&self) -> &Action {
        &self.action
    }

    /// A button labelled `label` that runs `action`.
    pub fn new(
        label: impl Into<String>,
        action: Action,
    ) -> Self {
        Self {
            label: label.into(),
            icon: None,
            action,
            confirm: None,
            enabled: None,
            disabled_note: None,
            note: None,
        }
    }

    /// Shows the Phosphor icon with this file stem before the label.
    #[must_use]
    pub fn icon(
        mut self,
        stem: impl Into<String>,
    ) -> Self {
        self.icon = Some(stem.into());
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

    /// Enables the button only while `condition` holds.
    #[must_use]
    pub fn enabled_when(
        mut self,
        condition: Condition,
    ) -> Self {
        self.enabled = Some(condition);
        self
    }

    /// The help line drawn under a button that stands between blocks.
    #[must_use]
    pub fn note(&self) -> Option<&str> {
        self.note.as_deref()
    }

    /// Sets the help line under a button that stands between blocks.
    #[must_use]
    pub fn noted(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.note = Some(text.into());
        self
    }

    /// Explains in the row description why the button is disabled.
    #[must_use]
    pub fn disabled_note(
        mut self,
        note: impl Into<String>,
    ) -> Self {
        self.disabled_note = Some(note.into());
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_fill_the_optional_parts() {
        let spec = ButtonSpec::new("Clear data", Action::backend("browser.clearData"))
            .icon("trash")
            .confirm(Confirm::new("Clear?", "Sure?", "Clear data"))
            .enabled_when(Condition::capability("browser.clearData"))
            .disabled_note("Not available")
            .noted("Help");
        assert_eq!(spec.icon.as_deref(), Some("trash"));
        assert!(spec.confirm.is_some() && spec.enabled.is_some());
        assert_eq!(spec.disabled_note.as_deref(), Some("Not available"));
        assert_eq!(spec.note(), Some("Help"));
    }
}
