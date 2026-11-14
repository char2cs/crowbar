//! The installed overlay host and the factory of overlay guards.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{App, Bounds, Global, Pixels};

use crate::overlay_guard::OverlayGuard;
use crate::overlay_host::OverlayHost;
use crate::overlay_id::OverlayId;

/// Global registry of the overlay host; without one overlays simply paint over the window.
#[derive(Clone)]
pub struct Overlays {
    host: Rc<dyn OverlayHost>,
    next: Rc<Cell<u64>>,
}

impl std::fmt::Debug for Overlays {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.debug_struct("Overlays").field("next", &self.next.get()).finish_non_exhaustive()
    }
}

impl Global for Overlays {}

impl Overlays {
    /// Installs `host` as the receiver of overlay notifications.
    pub fn install(host: Rc<dyn OverlayHost>, cx: &mut App) {
        cx.set_global(Self { host, next: Rc::new(Cell::new(1)) });
    }

    /// Announces a new overlay at `bounds`; the guard closes it when dropped.
    pub fn open(cx: &App, bounds: Bounds<Pixels>) -> OverlayGuard {
        let Some(overlays) = cx.try_global::<Self>() else {
            return OverlayGuard::detached();
        };
        let id = OverlayId(overlays.next.get());
        overlays.next.set(id.0 + 1);
        OverlayGuard::attach(Rc::clone(&overlays.host), id, bounds)
    }
}

#[cfg(test)]
#[path = "overlays_tests.rs"]
mod tests;
