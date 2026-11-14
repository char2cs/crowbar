use std::cell::RefCell;

use gpui::{Bounds, Pixels, point, px, size};

use super::*;

type Event = (u64, Option<Bounds<Pixels>>);

#[derive(Default)]
struct Recorder {
    events: RefCell<Vec<Event>>,
}

impl OverlayHost for Recorder {
    fn overlay_changed(&self, overlay: OverlayId, bounds: Bounds<Pixels>) {
        self.events.borrow_mut().push((overlay.get(), Some(bounds)));
    }

    fn overlay_closed(&self, overlay: OverlayId) {
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
