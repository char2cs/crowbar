pub(super) mod assignment;

use std::collections::HashMap;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use protocol::{PaneId, PaneOrigin};

use crate::scheme::origin_table::assignment::Assignment;

#[derive(Debug, Clone, Default)]
pub struct OriginTable {
    entries: Arc<Mutex<HashMap<webview::Id, Assignment>>>,
}

impl OriginTable {
    pub fn set(
        &self,
        view: webview::Id,
        pane: PaneId,
        origin: PaneOrigin,
    ) {
        self.lock().insert(view, Assignment { pane, origin });
    }

    pub fn remove(
        &self,
        view: webview::Id,
    ) {
        self.lock().remove(&view);
    }

    #[must_use]
    pub fn get(
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
    use protocol::{ExtensionId, PaneOrigin};
    use webview::ParentWindow;

    use super::*;
    use crate::test_support::blank_config;

    fn view_id() -> webview::Id {
        let host = webview::Host::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
        let config = blank_config(webview::Kind::Core);
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
