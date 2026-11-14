use super::*;
use crate::test_support::prelude::*;
use crate::visibility::Visibility;

use crate::dock_side::DockSide;

#[gpui::test]
fn an_open_sidebar_in_a_wide_window_renders_docked(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    cx.run_until_parked();
    let width = sidebar.read_with(cx, |this, _| this.live_width(1200.0));
    assert_eq!(width, 294.0);
}

#[gpui::test]
fn the_scrim_closes_a_drawer(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, Sidebar::dismiss_floating);
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.panel().visibility()),
        Visibility::Closed
    );
}

#[gpui::test]
fn a_right_dock_renders_without_panicking(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    sidebar.update(cx, |this, cx| this.dock_to(DockSide::Right, cx));
    cx.run_until_parked();
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.panel().side()),
        DockSide::Right
    );
}

#[gpui::test]
fn a_single_click_on_the_separator_starts_a_drag(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update_in(cx, |this, window, cx| {
        this.begin_resize(px(294.0), window, cx);
    });
    assert!(sidebar.read_with(cx, |this, _| this.resizing.is_some()));
}
