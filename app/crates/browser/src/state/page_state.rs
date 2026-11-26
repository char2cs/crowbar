use crate::state::page_update::PageUpdate;

/// What the browser chrome shows for one page: address, title and whether it is loading.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct PageState {
    url: String,
    title: Option<String>,
    loading: bool,
}

impl PageState {
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
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_navigation_runs_from_loading_to_loaded_and_forgets_the_old_title() {
        let mut state = PageState::default();
        state.apply(PageUpdate::title("Old"));
        state.apply(PageUpdate::Loading {
            url: "https://a.test/x".into(),
        });
        assert_eq!((state.is_loading(), state.title()), (true, None));
        state.apply(PageUpdate::title("A page"));
        state.apply(PageUpdate::Loaded {
            url: "https://a.test/x".into(),
        });
        assert_eq!(
            (state.url(), state.is_loading(), state.title()),
            ("https://a.test/x", false, Some("A page"))
        );
    }
}
