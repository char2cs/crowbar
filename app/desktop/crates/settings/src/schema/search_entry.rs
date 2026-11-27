#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct SearchEntry {
    pub(crate) section: String,
    pub(crate) label: String,
    pub(crate) description: String,
    pub(crate) keywords: Vec<String>,
}

impl SearchEntry {
    pub(crate) fn new(
        section: impl Into<String>,
        label: impl Into<String>,
        description: impl Into<String>,
    ) -> Self {
        Self {
            section: section.into(),
            label: label.into(),
            description: description.into(),
            keywords: Vec::new(),
        }
    }

    #[must_use]
    pub(crate) fn keywords(
        mut self,
        words: &[&str],
    ) -> Self {
        self.keywords = words.iter().map(|&word| word.to_owned()).collect();
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_fill_the_entry() {
        let entry = SearchEntry::new("Typography", "Font Size", "In pixels").keywords(&["font"]);
        assert_eq!(
            (
                entry.section.as_str(),
                entry.label.as_str(),
                entry.description.as_str()
            ),
            ("Typography", "Font Size", "In pixels")
        );
        assert_eq!(entry.keywords, ["font"]);
    }
}
