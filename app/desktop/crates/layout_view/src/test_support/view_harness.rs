use std::cell::RefCell;
use std::rc::Rc;

use gpui::{Entity, TestAppContext, VisualTestContext};
use model::{Command, Layout};
use ui::{Motion, Theme};

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::test_support::fake_source::FakeSource;

pub(crate) struct Harness<'a> {
    pub(crate) view: Entity<LayoutView>,
    pub(crate) cx: &'a mut VisualTestContext,
    pub(crate) commands: Rc<RefCell<Vec<Command>>>,
}

pub(crate) fn open<'a>(cx: &'a mut TestAppContext, layout: &Layout) -> Harness<'a> {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let (view, cx) = cx.add_window_view(|_, _| LayoutView::new(Rc::new(FakeSource::new())));
    let commands = Rc::new(RefCell::new(Vec::new()));
    let sink = Rc::clone(&commands);
    cx.update(|_, cx| {
        cx.subscribe(&view, move |_, event: &LayoutEvent, _| {
            let LayoutEvent::Apply(command) = event;
            sink.borrow_mut().push(command.clone());
        })
        .detach();
    });
    let tab = layout.active_tab().cloned();
    view.update(cx, |view, cx| view.set_tab(tab, cx));
    cx.run_until_parked();
    Harness { view, cx, commands }
}
