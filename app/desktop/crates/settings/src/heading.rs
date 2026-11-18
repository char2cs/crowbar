//! A page-level title with an explanation.

/// The large heading some pages draw above their body.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Heading {
    /// The large title text.
    pub title: String,
    /// The explanation under the title.
    pub description: String,
}

impl Heading {
    /// A heading with its description.
    pub fn new(
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
