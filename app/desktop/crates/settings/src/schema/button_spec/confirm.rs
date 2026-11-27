#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Confirm {
    pub(crate) title: String,
    pub(crate) message: String,
    pub(crate) action: String,
}

impl Confirm {
    #[must_use]
    pub fn action(&self) -> &str {
        &self.action
    }

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
