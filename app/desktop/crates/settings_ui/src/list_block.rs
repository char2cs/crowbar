//! A list of records the backend supplies.

/// Rows of live data (agents, sessions, hosts) shown under a source name.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ListBlock {
    pub(crate) source: String,
    pub(crate) title: Option<String>,
    pub(crate) intro: Option<String>,
    pub(crate) empty: String,
    pub(crate) loading: String,
    pub(crate) failed: String,
    pub(crate) columns: Vec<String>,
}

impl ListBlock {
    /// A list of the records the backend lists as `source`, showing `empty` when there are none.
    pub fn new(source: impl Into<String>, empty: impl Into<String>) -> Self {
        Self {
            source: source.into(),
            title: None,
            intro: None,
            empty: empty.into(),
            loading: "Loading...".to_owned(),
            failed: "Could not load this list.".to_owned(),
            columns: Vec::new(),
        }
    }

    /// A small heading above the list.
    #[must_use]
    pub fn titled(mut self, title: impl Into<String>) -> Self {
        self.title = Some(title.into());
        self
    }

    /// An explanation above the list, shown only over real records.
    #[must_use]
    pub fn intro(mut self, text: impl Into<String>) -> Self {
        self.intro = Some(text.into());
        self
    }

    /// Texts of the loading and failure states.
    #[must_use]
    pub fn states(mut self, loading: impl Into<String>, failed: impl Into<String>) -> Self {
        self.loading = loading.into();
        self.failed = failed.into();
        self
    }

    /// Labels of the switch columns, in order.
    #[must_use]
    pub fn columns(mut self, labels: &[&str]) -> Self {
        self.columns = labels.iter().map(|&label| label.to_owned()).collect();
        self
    }
}

#[cfg(test)]
#[path = "list_block_tests.rs"]
mod tests;
