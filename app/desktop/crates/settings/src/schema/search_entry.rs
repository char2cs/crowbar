//! One entry of a page's search index.

/// What the search matches a page by: a label, a description, a section title and keywords.
/// Spec: web/src/features/settings/config/search-index.ts.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SearchEntry {
    pub(crate) section: String,
    pub(crate) label: String,
    pub(crate) description: String,
    pub(crate) keywords: Vec<String>,
}

impl SearchEntry {
    /// An entry for the setting `label` in `section`, described by `description`.
    pub fn new(
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

    /// Words that find the entry without being shown.
    #[must_use]
    pub fn keywords(
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
