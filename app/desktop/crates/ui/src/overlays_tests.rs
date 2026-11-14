use std::cell::RefCell;

use gpui::{TestAppContext, point, px, size};

use super::*;

#[derive(Default)]
struct Recorder {
    events: RefCell<Vec<String>>,
}

impl OverlayHost for Recorder {
    fn overlay_changed(&self, overlay: OverlayId, bounds: Bounds<Pixels>) {
        self.events.borrow_mut().push(format!("changed {} {}", overlay.get(), f32::from(bounds.size.width)));
    }

    fn overlay_closed(&self, overlay: OverlayId) {
        self.events.borrow_mut().push(format!("closed {}", overlay.get()));
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
