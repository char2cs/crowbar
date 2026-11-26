//! Which origin each live web view currently acts for. The pane host writes it when it assigns a
//! pane to a web view; the protocol router reads it to authorize requests, so a page never names
//! its own identity.

pub(super) mod assignment;

use std::collections::HashMap;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::{PaneId, PaneOrigin};

use crate::scheme::origin_table::assignment::Assignment;

#[derive(Debug, Clone, Default)]
pub(crate) struct OriginTable {
    entries: Arc<Mutex<HashMap<webview::Id, Assignment>>>,
}

impl OriginTable {
    pub(crate) fn set(
        &self,
        view: webview::Id,
        pane: PaneId,
        origin: PaneOrigin,
    ) {
        self.lock().insert(view, Assignment { pane, origin });
    }

    pub(crate) fn remove(
        &self,
        view: webview::Id,
    ) {
        self.lock().remove(&view);
    }

    pub(crate) fn get(
        &self,
        view: webview::Id,
    ) -> Option<Assignment> {
        self.lock().get(&view).cloned()
    }

    fn lock(&self) -> MutexGuard<'_, HashMap<webview::Id, Assignment>> {
        self.entries.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

#[cfg(test)]
mod tests {
    use protocol::ExtensionId;
    use webview::ParentWindow;

    use super::*;
    use crate::test_support::blank_config;

    #[test]
    fn clones_share_one_table_whose_entries_are_replaced_and_removed() {
        let host = webview::Host::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
        let config = blank_config(webview::Kind::Core);
        let id = host.create_in(ParentWindow::X11(1), config).unwrap().id();
        let (table, pane) = (OriginTable::default(), PaneId::new("p1").unwrap());
        let copy = table.clone();
        copy.set(id, pane.clone(), PaneOrigin::Core);
        let origin = PaneOrigin::Extension(ExtensionId::new("git").unwrap());
        copy.set(id, pane.clone(), origin.clone());
        assert_eq!(table.get(id), Some(Assignment { pane, origin }), "replaced");
        table.remove(id);
        assert_eq!(copy.get(id), None, "removed");
    }
}
