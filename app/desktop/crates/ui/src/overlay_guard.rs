//! RAII handle of one open overlay.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{Bounds, Pixels};

use crate::overlay_host::OverlayHost;
use crate::overlay_id::OverlayId;

/// Keeps an overlay registered with the host; dropping it reports the close.
pub struct OverlayGuard {
    attached: Option<(Rc<dyn OverlayHost>, OverlayId)>,
    last: Cell<Option<Bounds<Pixels>>>,
}

impl std::fmt::Debug for OverlayGuard {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let id = self.attached.as_ref().map(|(_, id)| id.get());
        formatter.debug_struct("OverlayGuard").field("id", &id).finish_non_exhaustive()
    }
}

impl OverlayGuard {
    pub(crate) fn attach(host: Rc<dyn OverlayHost>, id: OverlayId, bounds: Bounds<Pixels>) -> Self {
        host.overlay_changed(id, bounds);
        Self { attached: Some((host, id)), last: Cell::new(Some(bounds)) }
    }

    pub(crate) fn detached() -> Self {
        Self { attached: None, last: Cell::new(None) }
    }

    /// Reports new bounds when they differ from the last report.
    pub fn update(&self, bounds: Bounds<Pixels>) {
        let Some((host, id)) = &self.attached else {
            return;
        };
        if self.last.get() == Some(bounds) {
            return;
        }
        self.last.set(Some(bounds));
        host.overlay_changed(*id, bounds);
    }
}

impl Drop for OverlayGuard {
    fn drop(&mut self) {
        if let Some((host, id)) = self.attached.take() {
            host.overlay_closed(id);
        }
    }
}

#[cfg(test)]
#[path = "overlay_guard_tests.rs"]
mod tests;
