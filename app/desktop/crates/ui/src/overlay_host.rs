//! The contract between overlays and the pane host.

use gpui::{Bounds, Pixels};

use crate::overlay_id::OverlayId;

/// Implemented by the pane host so web views never paint over menus, popovers and dialogs.
///
/// Native web views sit above GPUI content, so the host hides or clips every view that intersects the
/// bounds of an open overlay. Bounds are in window coordinates and are re-sent when the overlay moves
/// or resizes. A modal dialog reports the whole window.
pub trait OverlayHost {
    /// An overlay opened or changed its bounds.
    fn overlay_changed(
        &self,
        overlay: OverlayId,
        bounds: Bounds<Pixels>,
    );

    /// The overlay closed; views it hid may return.
    fn overlay_closed(
        &self,
        overlay: OverlayId,
    );
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{Bounds, Pixels, point, px, size};

    use super::*;

    type Event = (u64, Option<Bounds<Pixels>>);

    #[derive(Default)]
    struct Recorder {
        events: RefCell<Vec<Event>>,
    }

    impl OverlayHost for Recorder {
        fn overlay_changed(
            &self,
            overlay: OverlayId,
            bounds: Bounds<Pixels>,
        ) {
            self.events.borrow_mut().push((overlay.get(), Some(bounds)));
        }

        fn overlay_closed(
            &self,
            overlay: OverlayId,
        ) {
            self.events.borrow_mut().push((overlay.get(), None));
        }
    }

    #[test]
    fn a_host_sees_changes_then_the_close_of_the_same_overlay() {
        let host: &dyn OverlayHost = &Recorder::default();
        let bounds = Bounds::new(point(px(1.0), px(2.0)), size(px(3.0), px(4.0)));
        host.overlay_changed(OverlayId(7), bounds);
        host.overlay_closed(OverlayId(7));
        let recorder = Recorder::default();
        recorder.overlay_changed(OverlayId(7), bounds);
        recorder.overlay_closed(OverlayId(7));
        assert_eq!(
            *recorder.events.borrow(),
            vec![(7, Some(bounds)), (7, None)]
        );
    }
}
