use std::cell::{Cell, RefCell};
use std::rc::Rc;
use std::time::Instant;

use gpui::{App, Entity, TestAppContext, VisualTestContext};
use model::{Command, Layout};
use ui::{Motion, Theme};

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::test_support::fake_source::FakeSource;

pub(crate) struct Harness<'a> {
    pub(crate) view: Entity<LayoutView>,
    pub(crate) cx: &'a mut VisualTestContext,
    pub(crate) commands: Rc<RefCell<Vec<Command>>>,
    pub(crate) clock: Rc<Cell<Instant>>,
}

fn record_commands(
    view: &Entity<LayoutView>,
    sink: Rc<RefCell<Vec<Command>>>,
    cx: &mut App,
) {
    cx.subscribe(view, move |_, event: &LayoutEvent, _| {
        let LayoutEvent::Apply(command) = event;
        sink.borrow_mut().push(command.clone());
    })
    .detach();
}

pub(crate) fn open<'a>(
    cx: &'a mut TestAppContext,
    layout: &Layout,
) -> Harness<'a> {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let clock = Rc::new(Cell::new(Instant::now()));
    let reading = Rc::clone(&clock);
    let (view, cx) = cx.add_window_view(move |_, _| {
        let source = Rc::new(FakeSource::starting_at_first_tab());
        LayoutView::with_clock(source, Rc::new(move || reading.get()))
    });
    let commands = Rc::new(RefCell::new(Vec::new()));
    let sink = Rc::clone(&commands);
    cx.update(|_, cx| record_commands(&view, sink, cx));
    let tab = layout.active_tab().cloned();
    view.update(cx, |view, cx| view.set_tab(tab, cx));
    cx.run_until_parked();
    Harness {
        view,
        cx,
        commands,
        clock,
    }
}
