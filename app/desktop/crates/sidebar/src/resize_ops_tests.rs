use super::*;
use crate::test_support::prelude::*;

#[gpui::test]
fn dragging_changes_the_live_width_and_releasing_stores_it(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update_in(cx, |this, window, cx| {
        this.begin_resize(px(294.0), window, cx);
    });
    let event = MouseMoveEvent {
        position: point(px(400.0), px(300.0)),
        pressed_button: Some(gpui::MouseButton::Left),
        modifiers: gpui::Modifiers::none(),
    };
    sidebar.update(cx, |this, cx| this.move_resize(&event, cx));
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.live_width(1200.0)),
        400.0
    );
    sidebar.update(cx, Sidebar::end_resize);
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.panel().preferred()),
        400.0
    );
}

#[gpui::test]
fn dragging_below_half_the_minimum_collapses(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update_in(cx, |this, window, cx| {
        this.begin_resize(px(294.0), window, cx);
    });
    let event = MouseMoveEvent {
        position: point(px(50.0), px(300.0)),
        pressed_button: Some(gpui::MouseButton::Left),
        modifiers: gpui::Modifiers::none(),
    };
    sidebar.update(cx, |this, cx| this.move_resize(&event, cx));
    sidebar.update(cx, Sidebar::end_resize);
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.panel().visibility()),
        Visibility::Closed
    );
}

#[gpui::test]
fn the_home_key_collapses_and_end_widens(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let press = |key: &str| KeyDownEvent {
        keystroke: gpui::Keystroke::parse(key).expect("a key"),
        is_held: false,
    };
    sidebar.update_in(cx, |this, window, cx| {
        this.press_separator(&press("end"), window, cx);
    });
    assert!(sidebar.read_with(cx, |this, _| this.panel().preferred()) > 294.0);
    sidebar.update_in(cx, |this, window, cx| {
        this.press_separator(&press("home"), window, cx);
    });
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.panel().visibility()),
        Visibility::Closed
    );
}

#[gpui::test]
fn toggling_flips_between_open_and_closed(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, Sidebar::toggle_collapsed);
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.panel().visibility()),
        Visibility::Closed
    );
}
