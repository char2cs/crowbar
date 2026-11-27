#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Choice {
    pub(crate) value: String,
    pub(crate) label: String,
}

impl Choice {
    pub fn new(
        value: impl Into<String>,
        label: impl Into<String>,
    ) -> Self {
        Self {
            value: value.into(),
            label: label.into(),
        }
    }

    pub fn value(&self) -> &str {
        &self.value
    }

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
