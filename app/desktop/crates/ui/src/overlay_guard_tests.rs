use std::cell::RefCell;

use gpui::{point, px, size};

use super::*;

#[derive(Default)]
struct Recorder {
    events: RefCell<Vec<&'static str>>,
}

impl OverlayHost for Recorder {
    fn overlay_changed(&self, _: OverlayId, _: Bounds<Pixels>) {
        self.events.borrow_mut().push("changed");
    }

    fn overlay_closed(&self, _: OverlayId) {
        self.events.borrow_mut().push("closed");
    }
}

fn bounds(width: f32) -> Bounds<Pixels> {
    Bounds::new(point(px(0.0), px(0.0)), size(px(width), px(10.0)))
}

#[test]
fn update_only_reports_real_changes() {
    let recorder = Rc::new(Recorder::default());
    let guard = OverlayGuard::attach(Rc::clone(&recorder) as Rc<dyn OverlayHost>, OverlayId(7), bounds(10.0));
    guard.update(bounds(10.0));
    guard.update(bounds(11.0));
    drop(guard);
    assert_eq!(*recorder.events.borrow(), ["changed", "changed", "closed"]);
}
