//! A titled group of rows.

use crate::row::Row;

/// Rows under a heading; the first section of a page usually hides its heading.
#[derive(Debug, Clone, PartialEq)]
pub struct Section {
    pub(crate) title: String,
    pub(crate) description: Option<String>,
    pub(crate) show_header: bool,
    pub(crate) rows: Vec<Row>,
}

impl Section {
    /// A section titled `title` with a visible heading.
    pub fn new(title: impl Into<String>) -> Self {
        Self {
            title: title.into(),
            description: None,
            show_header: true,
            rows: Vec::new(),
        }
    }

    /// Hides the heading but keeps the title for search.
    #[must_use]
    pub fn headerless(mut self) -> Self {
        self.show_header = false;
        self
    }

    /// The text under the heading.
    #[must_use]
    pub fn describe(mut self, text: impl Into<String>) -> Self {
        self.description = Some(text.into());
        self
    }

    /// Appends a row.
    #[must_use]
    pub fn row(mut self, row: Row) -> Self {
        self.rows.push(row);
        self
    }
}

#[cfg(test)]
#[path = "section_tests.rs"]
mod tests;
