use protocol::HostId;
use webview::PageHandler;

/// Static settings of a [`PaneHost`](crate::PaneHost).
#[derive(Clone)]
pub struct Config {
    /// The custom scheme, without `://`.
    pub scheme: String,
    /// The daemon host panes talk to.
    pub host: HostId,
    /// Free core web views kept for reuse; more are destroyed.
    pub max_idle_slots: usize,
    /// Told about navigations in core and extension web views, e.g. to open links in the system browser.
    pub page_handler: Option<PageHandler>,
}

impl std::fmt::Debug for Config {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("Config")
            .field("scheme", &self.scheme)
            .field("host", &self.host)
            .field("max_idle_slots", &self.max_idle_slots)
            .field("page_handler", &self.page_handler.is_some())
            .finish()
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;

    fn config(page_handler: Option<PageHandler>) -> Config {
        Config {
            scheme: "crowbar".into(),
            host: HostId::new("local").unwrap(),
            max_idle_slots: 2,
            page_handler,
        }
    }

    #[test]
    fn debug_reports_whether_a_page_handler_is_set() {
        let handler: PageHandler = Arc::new(|_| true);
        assert!(
            format!("{:?}", config(Some(handler))).contains("page_handler: true"),
            "set"
        );
        assert!(
            format!("{:?}", config(None)).contains("page_handler: false"),
            "unset"
        );
    }

    #[test]
    fn clones_keep_the_page_handler() {
        let handler: PageHandler = Arc::new(|_| true);
        let copy = config(Some(Arc::clone(&handler))).clone();
        assert!(
            copy.page_handler
                .is_some_and(|kept| Arc::ptr_eq(&kept, &handler)),
            "same handler"
        );
    }
}
