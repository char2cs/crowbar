//! Fixtures shared by the unit tests: a strip in a test window that records what it asks for.
#![cfg(test)]

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, Entity, TestAppContext, VisualTestContext};
use layout::TabId;
use protocol::PaneId;
use ui::{Motion, Theme};

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

/// A strip whose window has a top layer, as the shell's has.
pub(crate) struct Floating {
    pub(crate) window: VisualTestContext,
    pub(crate) top: VisualTestContext,
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
    });
    let window = cx.add_window(|_, _| TabBar::new(TabId(50)));
    let bar = window.entity(cx).expect("the strip");
    let owner: gpui::AnyWindowHandle = window.into();
    let top = cx
        .update(|cx| owner.update(cx, |_, window, cx| ui::TopLayer::open(window, cx)))
        .expect("the strip window")
        .expect("the top layer");
    let mut window = VisualTestContext::from_window(owner, cx);
    bar.update(&mut window, |bar, cx| {
        bar.set_tabs(tabs, Some(TabId(active)), cx);
    });
    window.run_until_parked();
    Floating {
        window,
        top: VisualTestContext::from_window(top, cx),
    }
}
