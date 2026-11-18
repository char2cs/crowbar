//! Wording of a confirmation before a destructive action.

/// The question asked before an action runs.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Confirm {
    pub(crate) title: String,
    pub(crate) message: String,
    /// The label of the confirming button.
    pub action: String,
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
mod tests {
    use super::*;

    #[test]
    fn keeps_the_three_texts() {
        let confirm = Confirm::new("Clear?", "All of it.", "Clear data");
        assert_eq!(
            (
                confirm.title.as_str(),
                confirm.message.as_str(),
                confirm.action.as_str()
            ),
            ("Clear?", "All of it.", "Clear data")
        );
    }
}
