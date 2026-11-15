//! RAII handle of one open overlay.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{Bounds, Pixels};

use crate::overlay_host::OverlayHost;
use crate::overlay_id::OverlayId;

type Attachment = (Rc<dyn OverlayHost>, OverlayId);

/// Keeps an overlay registered with the host; dropping it reports the close.
pub struct OverlayGuard {
    attached: Option<Attachment>,
    last: Cell<Option<Bounds<Pixels>>>,
}

impl std::fmt::Debug for OverlayGuard {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        let id = self.attached.as_ref().map(|(_, id)| id.get());
        formatter
            .debug_struct("OverlayGuard")
            .field("id", &id)
            .finish_non_exhaustive()
    }
}

impl OverlayGuard {
    pub(crate) fn attach(
        host: Rc<dyn OverlayHost>,
        id: OverlayId,
        bounds: Bounds<Pixels>,
    ) -> Self {
        host.overlay_changed(id, bounds);
        Self {
            attached: Some((host, id)),
            last: Cell::new(Some(bounds)),
        }
    }

    pub(crate) fn detached() -> Self {
        Self {
            attached: None,
            last: Cell::new(None),
        }
    }

    /// Reports new bounds when they differ from the last report.
    pub fn update(
        &self,
        bounds: Bounds<Pixels>,
    ) {
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
mod tests {
    use std::cell::RefCell;

    use gpui::{point, px, size};

    use super::*;

    #[derive(Default)]
    struct Recorder {
        events: RefCell<Vec<&'static str>>,
    }

    impl OverlayHost for Recorder {
        fn overlay_changed(
            &self,
            _: OverlayId,
            _: Bounds<Pixels>,
        ) {
            self.events.borrow_mut().push("changed");
        }

        fn overlay_closed(
            &self,
            _: OverlayId,
        ) {
            self.events.borrow_mut().push("closed");
        }
    }

    fn bounds(width: f32) -> Bounds<Pixels> {
        Bounds::new(point(px(0.0), px(0.0)), size(px(width), px(10.0)))
    }

    #[test]
    fn update_only_reports_real_changes() {
        let recorder = Rc::new(Recorder::default());
        let guard = OverlayGuard::attach(
            Rc::clone(&recorder) as Rc<dyn OverlayHost>,
            OverlayId(7),
            bounds(10.0),
        );
        guard.update(bounds(10.0));
        guard.update(bounds(11.0));
        drop(guard);
        assert_eq!(*recorder.events.borrow(), ["changed", "changed", "closed"]);
    }
}
