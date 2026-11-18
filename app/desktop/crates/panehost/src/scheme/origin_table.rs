use std::collections::HashMap;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::{PaneId, PaneOrigin};
use webview::WebViewId;

use crate::scheme::assignment::Assignment;

/// Which origin each live web view currently acts for. The pane host writes it when it
/// assigns a pane to a web view; the protocol router reads it to authorize requests, so a
/// page never names its own identity.
#[derive(Debug, Clone, Default)]
pub struct OriginTable {
    entries: Arc<Mutex<HashMap<WebViewId, Assignment>>>,
}

impl OriginTable {
    /// Records which pane and origin a web view acts for, replacing any earlier entry.
    pub fn set(
        &self,
        view: WebViewId,
        pane: PaneId,
        origin: PaneOrigin,
    ) {
        self.lock().insert(view, Assignment { pane, origin });
    }

    /// Forgets a web view.
    pub fn remove(
        &self,
        view: WebViewId,
    ) {
        self.lock().remove(&view);
    }

    /// The assignment of a web view, if it has one.
    #[must_use]
    pub fn get(
        &self,
        view: WebViewId,
    ) -> Option<Assignment> {
        self.lock().get(&view).cloned()
    }

    fn lock(&self) -> MutexGuard<'_, HashMap<WebViewId, Assignment>> {
        self.entries.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
mod tests {
    use protocol::{ExtensionId, PaneOrigin};
    use webview::{ParentWindow, WebViewHost, WebViewKind};

    use super::*;
    use crate::test_support::blank_config;

    fn view_id() -> WebViewId {
        let host = WebViewHost::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
        let config = blank_config(WebViewKind::Core);
        host.create_in(ParentWindow::X11(1), config).unwrap().id()
    }

    #[test]
    fn set_replace_and_remove() {
        let table = OriginTable::default();
        let id = view_id();
        assert_eq!(table.get(id), None, "unknown view");
        let pane = PaneId::new("p1").unwrap();
        table.set(id, pane.clone(), PaneOrigin::Core);
        table.set(
            id,
            pane.clone(),
            PaneOrigin::Extension(ExtensionId::new("git").unwrap()),
        );
        assert_eq!(
            table.get(id),
            Some(Assignment {
                pane,
                origin: PaneOrigin::Extension(ExtensionId::new("git").unwrap())
            }),
            "replaced"
        );
        table.remove(id);
        assert_eq!(table.get(id), None, "removed");
    }

    #[test]
    fn clones_share_the_same_entries() {
        let table = OriginTable::default();
        let copy = table.clone();
        let id = view_id();
        copy.set(id, PaneId::new("p1").unwrap(), PaneOrigin::Core);
        assert!(table.get(id).is_some(), "visible through the other clone");
    }
}
