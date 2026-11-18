//! The installed overlay host and the factory of overlay guards.

mod overlay_slot;

pub(crate) use overlay_slot::OverlaySlot;

use std::cell::Cell;
use std::rc::Rc;

use gpui::{App, Bounds, Global, Pixels};

use crate::floating::overlay_guard::OverlayGuard;
use crate::floating::overlay_host::OverlayHost;
use crate::floating::overlay_id::OverlayId;

/// Global registry of the overlay host; without one overlays simply paint over the window.
#[derive(Clone)]
pub struct Overlays {
    host: Rc<dyn OverlayHost>,
    next: Rc<Cell<u64>>,
}

impl std::fmt::Debug for Overlays {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Overlays")
            .field("next", &self.next.get())
            .finish_non_exhaustive()
    }
}

impl Global for Overlays {}

impl Overlays {
    /// Installs `host` as the receiver of overlay notifications.
    pub fn install(
        host: Rc<dyn OverlayHost>,
        cx: &mut App,
    ) {
        cx.set_global(Self {
            host,
            next: Rc::new(Cell::new(1)),
        });
    }

    /// Announces a new overlay at `bounds`; the guard closes it when dropped.
    pub fn open(
        cx: &App,
        bounds: Bounds<Pixels>,
    ) -> OverlayGuard {
        let Some(overlays) = cx.try_global::<Self>() else {
            return OverlayGuard::detached();
        };
        let id = OverlayId(overlays.next.get());
        overlays.next.set(id.0 + 1);
        OverlayGuard::attach(Rc::clone(&overlays.host), id, bounds)
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{TestAppContext, point, px, size};

    use super::*;

    #[derive(Default)]
    struct Recorder {
        events: RefCell<Vec<String>>,
    }

    impl OverlayHost for Recorder {
        fn overlay_changed(
            &self,
            overlay: OverlayId,
            bounds: Bounds<Pixels>,
        ) {
            self.events.borrow_mut().push(format!(
                "changed {} {}",
                overlay.get(),
                f32::from(bounds.size.width)
            ));
        }

        fn overlay_closed(
            &self,
            overlay: OverlayId,
        ) {
            self.events
                .borrow_mut()
                .push(format!("closed {}", overlay.get()));
        }
    }

    fn bounds(width: f32) -> Bounds<Pixels> {
        Bounds::new(point(px(0.0), px(0.0)), size(px(width), px(10.0)))
    }

    #[gpui::test]
    fn open_reports_and_drop_closes(cx: &mut TestAppContext) {
        let recorder = Rc::new(Recorder::default());
        cx.update(|cx| Overlays::install(Rc::clone(&recorder) as Rc<dyn OverlayHost>, cx));
        let first = cx.update(|cx| Overlays::open(cx, bounds(100.0)));
        let second = cx.update(|cx| Overlays::open(cx, bounds(50.0)));
        drop(first);
        drop(second);
        assert_eq!(
            *recorder.events.borrow(),
            ["changed 1 100", "changed 2 50", "closed 1", "closed 2"],
        );
    }

    #[gpui::test]
    fn without_a_host_overlays_are_inert(cx: &mut TestAppContext) {
        let guard = cx.update(|cx| Overlays::open(cx, bounds(10.0)));
        guard.update(bounds(20.0));
        drop(guard);
    }
}
