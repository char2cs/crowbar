//! Fixtures shared by the unit tests: a strip in a test window that records what it asks for.
#![cfg(test)]

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{
    App, AppContext, Context, Entity, InteractiveElement, IntoElement, ParentElement, Render,
    StatefulInteractiveElement, Styled, TestAppContext, VisualTestContext, Window, div, px,
};
use layout::TabId;
use protocol::PaneId;
use ui::{DragGhost, DraggedRow, Motion, Theme};

use crate::event::Event;
use crate::pill::MemberView;
use crate::pill::TabView;
use crate::tab_bar::TabBar;

pub(crate) struct Harness<'a> {
    pub(crate) bar: Entity<TabBar>,
    pub(crate) cx: &'a mut VisualTestContext,
    pub(crate) events: Rc<RefCell<Vec<Event>>>,
}

pub(crate) fn view(
    id: u64,
    panes: &[&str],
) -> TabView {
    let members = panes
        .iter()
        .map(|name| MemberView {
            pane: PaneId::new(*name).expect("id"),
            label: (*name).to_owned().into(),
            provisional: false,
            working: false,
            kind: crate::pill::MemberKind::Chat,
            glyph: None,
        })
        .collect();
    TabView {
        id: TabId(id),
        members,
    }
}

fn record_events(
    bar: &Entity<TabBar>,
    sink: Rc<RefCell<Vec<Event>>>,
    cx: &mut App,
) {
    cx.subscribe(bar, move |_, event: &Event, _| {
        sink.borrow_mut().push(event.clone());
    })
    .detach();
}

pub(crate) fn open(
    cx: &mut TestAppContext,
    tabs: Vec<TabView>,
    active: u64,
) -> Harness<'_> {
    cx.update(|cx| {
        ui::init(cx).expect("ui init");
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let (bar, cx) = cx.add_window_view(|_, _| TabBar::new(TabId(50)));
    let events = Rc::new(RefCell::new(Vec::new()));
    let sink = Rc::clone(&events);
    cx.update(|_, cx| record_events(&bar, sink, cx));
    bar.update(cx, |bar, cx| bar.set_tabs(tabs, Some(TabId(active)), cx));
    cx.run_until_parked();
    Harness { bar, cx, events }
}

/// A strip in its window, whose floating elements open windows of their own.
pub(crate) struct Floating {
    pub(crate) window: VisualTestContext,
}

pub(crate) fn open_floating(
    cx: &mut TestAppContext,
    tabs: Vec<TabView>,
    active: u64,
) -> Floating {
    cx.update(|cx| {
        ui::init(cx).expect("ui init");
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
        ui::testing::open_float_windows(cx);
    });
    let window = cx.add_window(|_, _| TabBar::new(TabId(50)));
    let bar = window.entity(cx).expect("the strip");
    let owner: gpui::AnyWindowHandle = window.into();
    let mut window = VisualTestContext::from_window(owner, cx);
    bar.update(&mut window, |bar, cx| {
        bar.set_tabs(tabs, Some(TabId(active)), cx);
    });
    window.run_until_parked();
    Floating { window }
}

/// Where the row source sits: below the strip, in the window's left column.
pub(crate) const ROW_SOURCE_Y: f32 = 100.0;
const ROW_SOURCE_SIZE: f32 = 8.0;

struct RowSource {
    bar: Entity<TabBar>,
}

impl Render for RowSource {
    fn render(
        &mut self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let payload = DraggedRow {
            row: "t9".into(),
            chat: Some("t9".into()),
            file: None,
        };
        let source = div()
            .id("row-source")
            .absolute()
            .top(px(ROW_SOURCE_Y))
            .left_0()
            .size(px(ROW_SOURCE_SIZE))
            .on_drag(payload, |_, _, _, cx| {
                cx.new(|_| DragGhost::new("t9", ROW_SOURCE_SIZE))
            });
        div().size_full().child(self.bar.clone()).child(source)
    }
}

/// A strip in a window that also holds a sidebar chat row ("t9") to drag onto it.
pub(crate) fn open_under_row_source(
    cx: &mut TestAppContext,
    tabs: Vec<TabView>,
    active: u64,
) -> Harness<'_> {
    cx.update(|cx| {
        ui::init(cx).expect("ui init");
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let (host, cx) = cx.add_window_view(|_, cx| RowSource {
        bar: cx.new(|_| TabBar::new(TabId(50))),
    });
    let bar = host.read_with(cx, |host, _| host.bar.clone());
    let events = Rc::new(RefCell::new(Vec::new()));
    let sink = Rc::clone(&events);
    cx.update(|_, cx| record_events(&bar, sink, cx));
    bar.update(cx, |bar, cx| bar.set_tabs(tabs, Some(TabId(active)), cx));
    cx.run_until_parked();
    Harness { bar, cx, events }
}
