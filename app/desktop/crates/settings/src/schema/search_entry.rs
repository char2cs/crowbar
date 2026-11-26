//! One entry of a page's search index.

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
