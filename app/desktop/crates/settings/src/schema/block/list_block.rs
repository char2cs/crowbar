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
}

impl ListBlock {
    /// A small heading above the list, if any.
    #[must_use]
    pub fn title(&self) -> Option<&str> {
        self.title.as_deref()
    }

    /// The name the backend lists the records under.
    #[must_use]
    pub fn source(&self) -> &str {
        &self.source
    }

    /// The text shown while the records load.
    #[must_use]
    pub fn loading(&self) -> &str {
        &self.loading
    }

    /// An explanation above the list, shown only over real records.
    #[must_use]
    pub fn intro_text(&self) -> Option<&str> {
        self.intro.as_deref()
    }

    /// The text shown when the records could not be loaded.
    #[must_use]
    pub fn failed(&self) -> &str {
        &self.failed
    }

    /// The text shown when there are no records.
    #[must_use]
    pub fn empty(&self) -> &str {
        &self.empty
    }

    pub(crate) fn new(
        source: impl Into<String>,
        empty: impl Into<String>,
    ) -> Self {
        Self {
            source: source.into(),
            title: None,
            intro: None,
            empty: empty.into(),
            loading: "Loading...".to_owned(),
            failed: "Could not load this list.".to_owned(),
        }
    }

    #[must_use]
    pub(crate) fn titled(
        mut self,
        title: impl Into<String>,
    ) -> Self {
        self.title = Some(title.into());
        self
    }

    #[must_use]
    pub(crate) fn intro(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.intro = Some(text.into());
        self
    }

    #[must_use]
    pub(crate) fn states(
        mut self,
        loading: impl Into<String>,
        failed: impl Into<String>,
    ) -> Self {
        self.loading = loading.into();
        self.failed = failed.into();
        self
    }
}
