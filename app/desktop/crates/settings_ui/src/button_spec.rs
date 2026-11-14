//! A button in a row or between blocks.

use crate::action::Action;
use crate::condition::Condition;
use crate::confirm::Confirm;

/// A labelled button that runs an [`Action`], optionally after a confirmation.
#[derive(Debug, Clone, PartialEq)]
pub struct ButtonSpec {
    pub(crate) label: String,
    pub(crate) icon: Option<String>,
    pub(crate) action: Action,
    pub(crate) confirm: Option<Confirm>,
    pub(crate) enabled: Option<Condition>,
    pub(crate) disabled_note: Option<String>,
}

impl ButtonSpec {
    /// A button labelled `label` that runs `action`.
    pub fn new(label: impl Into<String>, action: Action) -> Self {
        Self {
            label: label.into(),
            icon: None,
            action,
            confirm: None,
            enabled: None,
            disabled_note: None,
        }
    }

    /// Shows the Phosphor icon with this file stem before the label.
    #[must_use]
    pub fn icon(mut self, stem: impl Into<String>) -> Self {
        self.icon = Some(stem.into());
        self
    }

    /// Asks `confirm` before the action runs.
    #[must_use]
    pub fn confirm(mut self, confirm: Confirm) -> Self {
        self.confirm = Some(confirm);
        self
    }

    /// Enables the button only while `condition` holds.
    #[must_use]
    pub fn enabled_when(mut self, condition: Condition) -> Self {
        self.enabled = Some(condition);
        self
    }

    /// Explains in the row description why the button is disabled.
    #[must_use]
    pub fn disabled_note(mut self, note: impl Into<String>) -> Self {
        self.disabled_note = Some(note.into());
        self
    }
}

#[cfg(test)]
#[path = "button_spec_tests.rs"]
mod tests;
