//! Wording of a confirmation before a destructive action.

/// The question asked before an action runs.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Confirm {
    pub(crate) title: String,
    pub(crate) message: String,
    pub(crate) action: String,
}

impl Confirm {
    /// A confirmation with a title, an explanation and the label of the confirming button.
    pub fn new(
        title: impl Into<String>,
        message: impl Into<String>,
        action: impl Into<String>,
    ) -> Self {
        Self {
            title: title.into(),
            message: message.into(),
            action: action.into(),
        }
    }
}

#[cfg(test)]
#[path = "confirm_tests.rs"]
mod tests;
