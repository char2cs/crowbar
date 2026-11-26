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
