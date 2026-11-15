use std::cell::RefCell;
use std::collections::HashSet;

use gpui::{Bounds, Pixels};
use ui::{OverlayHost, OverlayId};

use crate::request_queue::{Requests, send};
use crate::shell_request::ShellRequest;

#[derive(Debug)]
pub(crate) struct OverlayRequests {
    requests: Requests,
    open: RefCell<HashSet<OverlayId>>,
}

impl OverlayRequests {
    pub(crate) fn new(requests: Requests) -> Self {
        Self {
            requests,
            open: RefCell::new(HashSet::new()),
        }
    }

    fn report(&self) {
        let open = !self.open.borrow().is_empty();
        send(&self.requests, ShellRequest::Overlays { open });
    }
}

impl OverlayHost for OverlayRequests {
    fn overlay_changed(
        &self,
        overlay: OverlayId,
        _bounds: Bounds<Pixels>,
    ) {
        if self.open.borrow_mut().insert(overlay) {
            self.report();
        }
    }

    fn overlay_closed(
        &self,
        overlay: OverlayId,
    ) {
        if self.open.borrow_mut().remove(&overlay) {
            self.report();
        }
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{TestAppContext, point, px, size};
    use ui::Overlays;

    use super::*;

    fn area() -> Bounds<Pixels> {
        Bounds::new(point(px(0.0), px(0.0)), size(px(10.0), px(10.0)))
    }

    #[gpui::test]
    fn the_shell_is_told_whether_any_overlay_is_open(cx: &mut TestAppContext) {
        let (requests, queue) = async_channel::unbounded();
        cx.update(|cx| Overlays::install(Rc::new(OverlayRequests::new(requests)), cx));
        let first = cx.update(|cx| Overlays::open(cx, area()));
        let second = cx.update(|cx| Overlays::open(cx, area()));
        for _ in 0..2 {
            assert_eq!(
                queue.try_recv(),
                Ok(ShellRequest::Overlays { open: true }),
                "each new overlay reports that some are open"
            );
        }
        drop(first);
        assert_eq!(
            queue.try_recv(),
            Ok(ShellRequest::Overlays { open: true }),
            "one is still open"
        );
        drop(second);
        assert_eq!(
            queue.try_recv(),
            Ok(ShellRequest::Overlays { open: false }),
            "closed"
        );
    }
}
