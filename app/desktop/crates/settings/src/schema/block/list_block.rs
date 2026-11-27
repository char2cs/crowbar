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
    #[must_use]
    pub fn title(&self) -> Option<&str> {
        self.title.as_deref()
    }

    #[must_use]
    pub fn source(&self) -> &str {
        &self.source
    }

    #[must_use]
    pub fn loading(&self) -> &str {
        &self.loading
    }

    #[must_use]
    pub fn intro_text(&self) -> Option<&str> {
        self.intro.as_deref()
    }

    #[must_use]
    pub fn failed(&self) -> &str {
        &self.failed
    }

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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builders_override_the_defaults() {
        let list = ListBlock::new("agents", "No agents available.")
            .titled("Descriptor checks")
            .intro("Drag to reorder.")
            .states("Loading agents...", "Could not load agents.");
        assert_eq!(list.source, "agents");
        assert_eq!(list.title.as_deref(), Some("Descriptor checks"));
        assert_eq!(list.intro.as_deref(), Some("Drag to reorder."));
        assert_eq!(
            (list.loading.as_str(), list.failed.as_str()),
            ("Loading agents...", "Could not load agents.")
        );
    }
}
