use crate::schema::row::Row;

#[derive(Debug, Clone, PartialEq)]
pub struct Section {
    pub(crate) title: String,
    pub(crate) description: Option<String>,
    pub(crate) show_header: bool,
    pub(crate) rows: Vec<Row>,
    pub(crate) note: Option<String>,
}

impl Section {
    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    #[must_use]
    pub fn show_header(&self) -> bool {
        self.show_header
    }

    #[must_use]
    pub fn rows(&self) -> &[Row] {
        &self.rows
    }

    #[must_use]
    pub fn description(&self) -> Option<&str> {
        self.description.as_deref()
    }

    #[must_use]
    pub fn note(&self) -> Option<&str> {
        self.note.as_deref()
    }

    pub(crate) fn new(title: impl Into<String>) -> Self {
        Self {
            title: title.into(),
            description: None,
            show_header: true,
            rows: Vec::new(),
            note: None,
        }
    }

    #[must_use]
    pub(crate) fn headerless(mut self) -> Self {
        self.show_header = false;
        self
    }

    #[must_use]
    pub(crate) fn describe(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.description = Some(text.into());
        self
    }

    #[must_use]
    pub(crate) fn noted(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.note = Some(text.into());
        self
    }

    #[must_use]
    pub(crate) fn row(
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
    use crate::schema::control::Control;

    #[test]
    fn headerless_keeps_the_title_and_rows_keep_their_order() {
        let section = Section::new("Theme")
            .headerless()
            .describe("Colours")
            .noted("Later")
            .row(Row::new("a", "A", Control::switch()))
            .row(Row::new("b", "B", Control::switch()));
        assert!(!section.show_header);
        assert_eq!(section.title, "Theme");
        assert_eq!(section.description.as_deref(), Some("Colours"));
        assert_eq!(section.note(), Some("Later"));
        let keys: Vec<&str> = section.rows.iter().map(|row| row.key().as_str()).collect();
        assert_eq!(keys, ["a", "b"]);
    }
}
