use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, Entity, TestAppContext, VisualTestContext};
use model::TabId;
use protocol::PaneId;
use ui::{Motion, Theme};

use crate::member_view::MemberView;
use crate::tab_bar::TabBar;
use crate::tab_bar_event::TabBarEvent;
use crate::tab_view::TabView;

pub(crate) struct Harness<'a> {
    pub(crate) bar: Entity<TabBar>,
    pub(crate) cx: &'a mut VisualTestContext,
    pub(crate) events: Rc<RefCell<Vec<TabBarEvent>>>,
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
        })
        .collect();
    TabView {
        id: TabId(id),
        members,
    }
}

fn record_events(
    bar: &Entity<TabBar>,
    sink: Rc<RefCell<Vec<TabBarEvent>>>,
    cx: &mut App,
) {
    cx.subscribe(bar, move |_, event: &TabBarEvent, _| {
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
