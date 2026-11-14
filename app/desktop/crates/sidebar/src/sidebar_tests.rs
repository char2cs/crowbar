use super::*;
use crate::dock_side::DockSide;
use crate::test_support::prelude::*;

#[gpui::test]
fn changes_to_the_geometry_are_reported_as_preferences(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let events = Rc::new(std::cell::Cell::new(0));
    let seen = Rc::clone(&events);
    let _subscription = cx.update(|_, cx| {
        cx.subscribe(&sidebar, move |_, _: &SidebarEvent, _| {
            seen.set(seen.get() + 1);
        })
    });
    sidebar.update(cx, |this, cx| this.dock_to(DockSide::Right, cx));
    assert_eq!(events.get(), 1);
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.preferences().side),
        DockSide::Right
    );
}

#[gpui::test]
fn each_space_keeps_its_own_scroll_position(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    let (a, b) = sidebar.update(cx, |this, _| {
        (
            this.scroll_for(&ProjectId::from("inv")),
            this.scroll_for(&ProjectId::from("other")),
        )
    });
    a.set_offset(point(px(0.0), px(-10.0)));
    assert_ne!(a.offset(), b.offset());
}

#[gpui::test]
fn toggling_twice_restores_the_sidebar(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let before = sidebar.read_with(cx, |this, _| this.panel().visibility());
    sidebar.update(cx, |this, cx| {
        this.toggle(cx);
        this.toggle(cx);
    });
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.panel().visibility()),
        before
    );
}
