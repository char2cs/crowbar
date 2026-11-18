//! One option of a select.

/// A value a select can store and the label it shows.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Choice {
    pub(crate) value: String,
    pub(crate) label: String,
}

impl Choice {
    /// A choice storing `value` and showing `label`.
    pub fn new(
        value: impl Into<String>,
        label: impl Into<String>,
    ) -> Self {
        Self {
            value: value.into(),
            label: label.into(),
        }
    }

    /// The stored value.
    pub fn value(&self) -> &str {
        &self.value
    }

    /// The shown label.
    pub fn label(&self) -> &str {
        &self.label
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn keeps_value_and_label_apart() {
        let choice = Choice::new("system", "Sync with System");
        assert_eq!(
            (choice.value(), choice.label()),
            ("system", "Sync with System")
        );
    }
}
