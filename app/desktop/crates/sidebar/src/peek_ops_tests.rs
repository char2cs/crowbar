use super::*;
use crate::test_support::prelude::*;

fn closed_sidebar(
    cx: &mut TestAppContext,
) -> (gpui::Entity<Sidebar>, &mut gpui::VisualTestContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, Sidebar::toggle);
    (sidebar, cx)
}

#[gpui::test]
fn the_pointer_on_the_left_edge_summons_the_card(cx: &mut TestAppContext) {
    let (sidebar, cx) = closed_sidebar(cx);
    sidebar.update_in(cx, |this, window, cx| {
        this.track_peek(point(px(2.0), px(300.0)), window, cx);
    });
    assert!(sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
}

#[gpui::test]
fn leaving_the_card_dismisses_it(cx: &mut TestAppContext) {
    let (sidebar, cx) = closed_sidebar(cx);
    sidebar.update_in(cx, |this, window, cx| {
        this.track_peek(point(px(2.0), px(300.0)), window, cx);
        this.track_peek(point(px(900.0), px(300.0)), window, cx);
    });
    assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
}

#[gpui::test]
fn an_open_sidebar_ignores_peeking(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, Sidebar::begin_peek);
    assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
}

#[gpui::test]
fn the_shell_can_begin_and_end_a_peek(cx: &mut TestAppContext) {
    let (sidebar, cx) = closed_sidebar(cx);
    sidebar.update(cx, Sidebar::begin_peek);
    assert!(sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    sidebar.update(cx, Sidebar::end_peek);
    assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
}
