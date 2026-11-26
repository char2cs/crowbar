use std::cell::{Cell, RefCell};
use std::rc::Rc;
use std::time::Instant;

use gpui::{
    App, AppContext, Context, Entity, InteractiveElement, IntoElement, Modifiers, MouseButton,
    ParentElement, Pixels, Point, Render, StatefulInteractiveElement, Styled, TestAppContext,
    VisualTestContext, Window, div, point, px,
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
        LayoutEvent::Hovered => {},
        other => opened.borrow_mut().push(other.clone()),
    })
    .detach();
}

struct RowSource {
    view: Entity<LayoutView>,
    payload: DraggedRow,
}

/// The payload of the sidebar's row for chat `t1`.
pub(crate) fn chat_row() -> DraggedRow {
    DraggedRow {
        row: "t1".into(),
        chat: Some("t1".into()),
        file: None,
    }
}

/// The payload of the Files panel's row for the file at `path`.
pub(crate) fn file_row(path: &str) -> DraggedRow {
    DraggedRow {
        row: path.into(),
        chat: None,
        file: Some(path.into()),
    }
}

const SOURCE_SIZE: f32 = 8.0;

impl Render for RowSource {
    fn render(
        &mut self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let payload = self.payload.clone();
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
    let (clock, reading) = init(cx);
    let (view, cx) = cx.add_window_view(move |_, _| {
        LayoutView::with_clock(Rc::new(source), Rc::new(move || reading.get()))
    });
    harness(view, cx, layout, clock)
}

pub(crate) fn open_under_row_source<'a>(
    cx: &'a mut TestAppContext,
    layout: &Layout,
    payload: DraggedRow,
) -> Harness<'a> {
    let (clock, reading) = init(cx);
    let (host, cx) = cx.add_window_view(move |_, cx| {
        let source = Rc::new(FakeSource::starting_at_first_tab());
        let view = cx.new(|_| LayoutView::with_clock(source, Rc::new(move || reading.get())));
        RowSource { view, payload }
    });
    let view = host.read_with(cx, |host, _| host.view.clone());
    harness(view, cx, layout, clock)
}

/// Drags the row source's row from its corner to `to` and lets it go there.
pub(crate) fn drag_row_to(
    cx: &mut VisualTestContext,
    to: Point<Pixels>,
) {
    let (none, left) = (Modifiers::none(), MouseButton::Left);
    cx.simulate_mouse_down(point(px(4.0), px(4.0)), left, none);
    cx.simulate_mouse_move(point(px(40.0), px(40.0)), left, none);
    cx.simulate_mouse_move(to, left, none);
    cx.simulate_mouse_up(to, left, none);
}

type Clocks = (Rc<Cell<Instant>>, Rc<Cell<Instant>>);

fn init(cx: &mut TestAppContext) -> Clocks {
    cx.update(|cx| {
        ui::init(cx).expect("ui init");
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let clock = Rc::new(Cell::new(Instant::now()));
    (Rc::clone(&clock), clock)
}

fn harness<'a>(
    view: Entity<LayoutView>,
    cx: &'a mut VisualTestContext,
    layout: &Layout,
    clock: Rc<Cell<Instant>>,
) -> Harness<'a> {
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

/// A layout view in a visual window, where drags, drop previews and the handle are drawn.
pub(crate) struct Floating {
    pub(crate) view: Entity<LayoutView>,
    pub(crate) window: VisualTestContext,
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
    let (clock, reading) = init(cx);
    cx.update(ui::testing::open_float_windows);
    let window = cx.add_window(move |_, _| {
        LayoutView::with_clock(Rc::new(source), Rc::new(move || reading.get()))
    });
    let view = window
        .entity(cx)
        .unwrap_or_else(|error| panic!("the view: {error}"));
    let owner: gpui::AnyWindowHandle = window.into();
    let mut window = VisualTestContext::from_window(owner, cx);
    let tab = layout.showing("").cloned();
    view.update(&mut window, |view, cx| view.set_tab(tab, cx));
    window.run_until_parked();
    Floating {
        view,
        window,
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
