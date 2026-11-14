use std::cell::{Cell, RefCell};
use std::rc::Rc;

use gpui::{Bounds, ParentElement, Pixels, TestAppContext, div, point, px};

use super::*;
use crate::overlay_host::OverlayHost;
use crate::overlay_id::OverlayId;
use crate::test_support::open;

#[derive(Default)]
struct Recorder {
    events: RefCell<Vec<String>>,
}

impl OverlayHost for Recorder {
    fn overlay_changed(&self, _: OverlayId, bounds: Bounds<Pixels>) {
        self.events.borrow_mut().push(format!(
            "changed {}x{}",
            f32::from(bounds.size.width),
            f32::from(bounds.size.height)
        ));
    }

    fn overlay_closed(&self, _: OverlayId) {
        self.events.borrow_mut().push("closed".into());
    }
}

#[gpui::test]
fn dialog_reports_the_whole_window_to_the_host(cx: &mut TestAppContext) {
    let recorder = Rc::new(Recorder::default());
    let host = Rc::clone(&recorder);
    let cx = open(cx, |_, _| Dialog::new("d").title("Title"));
    cx.update(|_, cx| Overlays::install(host as Rc<dyn OverlayHost>, cx));
    cx.simulate_resize(gpui::size(px(640.0), px(480.0)));
    cx.run_until_parked();
    let events = recorder.events.borrow();
    assert!(
        events.iter().all(|event| event.starts_with("changed")),
        "{events:?}"
    );
}

#[gpui::test]
fn close_button_and_scrim_both_call_on_close(cx: &mut TestAppContext) {
    let closed = Rc::new(Cell::new(0));
    let counter = Rc::clone(&closed);
    let cx = open(cx, move |_, _| {
        let counter = Rc::clone(&counter);
        Dialog::new("d")
            .title("Title")
            .body("Body")
            .on_close(move |_, _| counter.set(counter.get() + 1))
    });
    cx.simulate_click(point(px(2.0), px(2.0)), gpui::Modifiers::none());
    assert_eq!(closed.get(), 1, "a press on the scrim closes");
}

#[gpui::test]
fn a_press_inside_the_popup_does_not_close(cx: &mut TestAppContext) {
    let closed = Rc::new(Cell::new(0));
    let counter = Rc::clone(&closed);
    let cx = open(cx, move |_, _| {
        let counter = Rc::clone(&counter);
        Dialog::new("d")
            .title("Title")
            .body(div().debug_selector(|| "body".into()).child("Body"))
            .on_close(move |_, _| counter.set(counter.get() + 1))
    });
    let body = cx.debug_bounds("body").unwrap_or_default();
    cx.simulate_click(body.center(), gpui::Modifiers::none());
    assert_eq!(closed.get(), 0);
}

#[gpui::test]
fn a_flush_dialog_fills_its_extent_with_no_body_padding(cx: &mut TestAppContext) {
    let cx = open(cx, |_, _| {
        Dialog::new("d")
            .title("Title")
            .flush(DialogExtent::new(0.5, 0.5, px(300.0), px(200.0)))
            .body(div().size_full().debug_selector(|| "body".into()))
    });
    let popup = cx.debug_bounds("body").unwrap_or_default();
    assert_eq!(
        f32::from(popup.size.width),
        300.0,
        "the body spans the popup width: no padding"
    );
}
