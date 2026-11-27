#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Heading {
    pub(crate) title: String,
    pub(crate) description: String,
}

impl Heading {
    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    #[must_use]
    pub fn description(&self) -> &str {
        &self.description
    }

    pub(crate) fn new(
        title: impl Into<String>,
        description: impl Into<String>,
    ) -> Self {
        Self {
            title: title.into(),
            description: description.into(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn keeps_title_and_description() {
        let heading = Heading::new("Hosts", "Add another machine.");
        assert_eq!(
            (heading.title.as_str(), heading.description.as_str()),
            ("Hosts", "Add another machine.")
        );
    }
}
