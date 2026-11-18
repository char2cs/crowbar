//! A titled group of rows.

use crate::row::Row;

/// Rows under a heading; the first section of a page usually hides its heading.
#[derive(Debug, Clone, PartialEq)]
pub struct Section {
    /// The heading text, also used for search when the heading is hidden.
    pub title: String,
    /// The text under the heading, if any.
    pub description: Option<String>,
    /// Whether the heading is drawn.
    pub show_header: bool,
    /// The rows of the section, in order.
    pub rows: Vec<Row>,
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
    pub fn describe(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.description = Some(text.into());
        self
    }

    /// Appends a row.
    #[must_use]
    pub fn row(
        mut self,
        row: Row,
    ) -> Self {
        self.rows.push(row);
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::control::Control;

    #[test]
    fn headerless_keeps_the_title_and_rows_keep_their_order() {
        let section = Section::new("Theme")
            .headerless()
            .describe("Colours")
            .row(Row::new("a", "A", Control::switch()))
            .row(Row::new("b", "B", Control::switch()));
        assert!(!section.show_header);
        assert_eq!(section.title, "Theme");
        assert_eq!(section.description.as_deref(), Some("Colours"));
        let keys: Vec<&str> = section.rows.iter().map(|row| row.key().as_str()).collect();
        assert_eq!(keys, ["a", "b"]);
    }
}
