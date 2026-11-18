use crate::state::page_update::PageUpdate;
use crate::state::tab_name::tab_name;

/// What the browser chrome shows for one page: address, title and whether it is loading.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct PageState {
    url: String,
    title: Option<String>,
    loading: bool,
}

impl PageState {
    /// A page that has not loaded yet, showing `url` in the address bar.
    #[must_use]
    pub fn at(url: &str) -> Self {
        Self {
            url: url.to_owned(),
            ..Self::default()
        }
    }

    /// Folds one engine report into the state. A new navigation drops the old title.
    pub fn apply(
        &mut self,
        update: PageUpdate,
    ) {
        match update {
            PageUpdate::Loading { url } => {
                self.url = url;
                self.title = None;
                self.loading = true;
            },
            PageUpdate::Loaded { url } => {
                self.url = url;
                self.loading = false;
            },
            PageUpdate::Title { title } => self.title = Some(title),
            PageUpdate::Picked { .. }
            | PageUpdate::PickEnded
            | PageUpdate::PickFailed { .. }
            | PageUpdate::AgentReply { .. } => {},
        }
    }

    /// The address shown in the address bar.
    #[must_use]
    pub fn url(&self) -> &str {
        &self.url
    }

    /// Whether the reload button shows a stop symbol.
    #[must_use]
    pub fn is_loading(&self) -> bool {
        self.loading
    }

    /// The page title, once the page has set one.
    #[must_use]
    pub fn title(&self) -> Option<&str> {
        self.title.as_deref()
    }

    /// The tab label: the title, else the host, else a placeholder.
    #[must_use]
    pub fn name(&self) -> String {
        tab_name(self.title.as_deref(), &self.url)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_navigation_runs_from_loading_to_loaded() {
        let mut state = PageState::at("");
        state.apply(PageUpdate::Loading {
            url: "https://a.test/x".into(),
        });
        assert!(state.is_loading());
        state.apply(PageUpdate::title("A page"));
        state.apply(PageUpdate::Loaded {
            url: "https://a.test/x".into(),
        });
        assert_eq!(
            (state.url(), state.is_loading(), state.name().as_str()),
            ("https://a.test/x", false, "A page")
        );
    }

    #[test]
    fn a_title_belongs_to_its_page_and_a_new_navigation_forgets_it() {
        let mut state = PageState::at("https://a.test");
        assert_eq!(state.title(), None);
        state.apply(PageUpdate::title("Old"));
        assert_eq!(state.title(), Some("Old"));
        state.apply(PageUpdate::Loading {
            url: "https://b.test".into(),
        });
        assert_eq!((state.title(), state.name().as_str()), (None, "b.test"));
    }
}
