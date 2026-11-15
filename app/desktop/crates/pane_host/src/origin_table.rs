use std::collections::HashMap;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::PaneOrigin;
use webview::WebViewId;

/// Which origin each live web view currently acts for. The pane host writes it when it
/// assigns a pane to a web view; the protocol router reads it to authorize requests, so a
/// page never names its own identity.
#[derive(Debug, Clone, Default)]
pub struct OriginTable {
    entries: Arc<Mutex<HashMap<WebViewId, PaneOrigin>>>,
}

impl OriginTable {
    /// Records the origin of a web view, replacing any earlier one.
    pub fn set(&self, view: WebViewId, origin: PaneOrigin) {
        self.lock().insert(view, origin);
    }

    /// Forgets a web view.
    pub fn remove(&self, view: WebViewId) {
        self.lock().remove(&view);
    }

    /// The origin of a web view, if it has one.
    #[must_use]
    pub fn get(&self, view: WebViewId) -> Option<PaneOrigin> {
        self.lock().get(&view).cloned()
    }

    fn lock(&self) -> MutexGuard<'_, HashMap<WebViewId, PaneOrigin>> {
        self.entries.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
#[path = "origin_table_tests.rs"]
mod tests;
