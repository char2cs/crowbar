use std::cell::{Cell, RefCell};
use std::rc::Rc;
use std::time::Instant;

use gpui::{
    App, AppContext, Context, Entity, InteractiveElement, IntoElement, ParentElement, Render,
    StatefulInteractiveElement, Styled, TestAppContext, VisualTestContext, Window, div, px,
};
use layout::{Command, Layout};
use ui::{DragGhost, DraggedRow, Motion, Theme};

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::test_support::fake_source::FakeSource;
use ideshell::PaneStrip;
use protocol::PaneId;

pub(crate) struct Harness<'a> {
    pub(crate) view: Entity<LayoutView>,
    pub(crate) cx: &'a mut VisualTestContext,
    pub(crate) commands: Rc<RefCell<Vec<Command>>>,
    pub(crate) opened: Rc<RefCell<Vec<LayoutEvent>>>,
    pub(crate) clock: Rc<Cell<Instant>>,
}

type Sinks = (Rc<RefCell<Vec<Command>>>, Rc<RefCell<Vec<LayoutEvent>>>);

fn record_events(
    view: &Entity<LayoutView>,
    (commands, opened): Sinks,
    cx: &mut App,
) {
    cx.subscribe(view, move |_, event: &LayoutEvent, _| match event {
        LayoutEvent::Apply(command) => commands.borrow_mut().push(command.clone()),
        other => opened.borrow_mut().push(other.clone()),
    })
    .detach();
}

struct RowSource {
    view: Entity<LayoutView>,
}

const SOURCE_SIZE: f32 = 8.0;

impl Render for RowSource {
    fn render(
        &mut self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let payload = DraggedRow {
            row: "t1".into(),
            chat: Some("t1".into()),
        };
        let source = div()
            .id("row-source")
            .absolute()
            .top_0()
            .left_0()
            .size(px(SOURCE_SIZE))
            .on_drag(payload, |_, _, _, cx| {
                cx.new(|_| DragGhost::new("t1", SOURCE_SIZE))
            });
        div().size_full().child(self.view.clone()).child(source)
    }
}

pub(crate) fn open<'a>(
    cx: &'a mut TestAppContext,
    layout: &Layout,
) -> Harness<'a> {
    open_with(cx, layout, FakeSource::starting_at_first_tab())
}

pub(crate) fn open_with<'a>(
    cx: &'a mut TestAppContext,
    layout: &Layout,
    source: FakeSource,
) -> Harness<'a> {
    cx.update(|cx| {
        ui::init(cx).expect("ui init");
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let clock = Rc::new(Cell::new(Instant::now()));
    let reading = Rc::clone(&clock);
    let (view, cx) = cx.add_window_view(move |_, _| {
        LayoutView::with_clock(Rc::new(source), Rc::new(move || reading.get()))
    });
    let commands = Rc::new(RefCell::new(Vec::new()));
    let opened = Rc::new(RefCell::new(Vec::new()));
    let sinks = (Rc::clone(&commands), Rc::clone(&opened));
    cx.update(|_, cx| record_events(&view, sinks, cx));
    let tab = layout.showing("").cloned();
    view.update(cx, |view, cx| view.set_tab(tab, cx));
    cx.run_until_parked();
    Harness {
        view,
        cx,
        commands,
        opened,
        clock,
    }
}

pub(crate) fn open_under_row_source<'a>(
    cx: &'a mut TestAppContext,
    layout: &Layout,
) -> Harness<'a> {
    cx.update(|cx| {
        ui::init(cx).expect("ui init");
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let clock = Rc::new(Cell::new(Instant::now()));
    let reading = Rc::clone(&clock);
    let (host, cx) = cx.add_window_view(move |_, cx| {
        let source = Rc::new(FakeSource::starting_at_first_tab());
        let view = cx.new(|_| LayoutView::with_clock(source, Rc::new(move || reading.get())));
        RowSource { view }
    });
    let view = host.read_with(cx, |host, _| host.view.clone());
    let commands = Rc::new(RefCell::new(Vec::new()));
    let opened = Rc::new(RefCell::new(Vec::new()));
    let sinks = (Rc::clone(&commands), Rc::clone(&opened));
    cx.update(|_, cx| record_events(&view, sinks, cx));
    let tab = layout.showing("").cloned();
    view.update(cx, |view, cx| view.set_tab(tab, cx));
    cx.run_until_parked();
    Harness {
        view,
        cx,
        commands,
        opened,
        clock,
    }
}

/// A layout view in a window with a top layer, which shows what floats above the panes.
pub(crate) struct Floating {
    pub(crate) view: Entity<LayoutView>,
    pub(crate) window: VisualTestContext,
    pub(crate) top: VisualTestContext,
    pub(crate) clock: Rc<Cell<Instant>>,
}

pub(crate) fn open_floating(
    cx: &mut TestAppContext,
    layout: &Layout,
) -> Floating {
    open_floating_with(cx, layout, FakeSource::starting_at_first_tab())
}

pub(crate) fn open_floating_with(
    cx: &mut TestAppContext,
    layout: &Layout,
    source: FakeSource,
) -> Floating {
    cx.update(|cx| {
        ui::init(cx).expect("ui init");
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let clock = Rc::new(Cell::new(Instant::now()));
    let reading = Rc::clone(&clock);
    let window = cx.add_window(move |_, _| {
        LayoutView::with_clock(Rc::new(source), Rc::new(move || reading.get()))
    });
    let view = window
        .entity(cx)
        .unwrap_or_else(|error| panic!("the view: {error}"));
    let owner: gpui::AnyWindowHandle = window.into();
    let top = cx.update(|cx| owner.update(cx, |_, window, cx| ui::TopLayer::open(window, cx)));
    let top = top.unwrap_or_else(|error| panic!("the layout window: {error}"));
    let top = top.unwrap_or_else(|error| panic!("the top layer: {error}"));
    let mut window = VisualTestContext::from_window(owner, cx);
    let tab = layout.showing("").cloned();
    view.update(&mut window, |view, cx| view.set_tab(tab, cx));
    window.run_until_parked();
    Floating {
        view,
        window,
        top: VisualTestContext::from_window(top, cx),
        clock,
    }
}

pub(crate) fn strip_of(
    view: &LayoutView,
    pane: &PaneId,
    cx: &mut Context<'_, LayoutView>,
) -> Option<PaneStrip> {
    let group = view.tab.as_ref()?.group_of(pane)?;
    view.strip_for(group, cx)
}
