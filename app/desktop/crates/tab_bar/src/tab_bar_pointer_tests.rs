use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
use model::{Command, TabId};

use crate::tab_bar_event::TabBarEvent;
use crate::test_support::{open, view};

fn tabs() -> Vec<crate::tab_view::TabView> {
    vec![view(1, &["a"]), view(2, &["b"]), view(3, &["c"])]
}

fn applied(events: &[TabBarEvent]) -> Vec<Vec<Command>> {
    events
        .iter()
        .filter_map(|event| match event {
            TabBarEvent::Apply(commands) => Some(commands.clone()),
            _ => None,
        })
        .collect()
}

#[gpui::test]
fn clicking_a_tab_activates_it(cx: &mut TestAppContext) {
    let harness = open(cx, tabs(), 1);
    let (cx, events) = (harness.cx, harness.events);
    cx.simulate_click(point(px(8.0 + 240.0 + 120.0), px(21.0)), Modifiers::none());
    assert_eq!(
        applied(&events.borrow()),
        vec![vec![Command::ActivateTab { tab: TabId(2) }]],
        "tab two"
    );
}

#[gpui::test]
fn dragging_a_tab_past_its_neighbour_reorders_on_release(cx: &mut TestAppContext) {
    let harness = open(cx, tabs(), 1);
    let (cx, events) = (harness.cx, harness.events);
    let at = |x: f32| point(px(x), px(21.0));
    cx.simulate_mouse_down(at(8.0 + 100.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_move(at(8.0 + 160.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_move(at(8.0 + 330.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_up(at(8.0 + 330.0), MouseButton::Left, Modifiers::none());
    assert_eq!(
        applied(&events.borrow()),
        vec![vec![Command::MoveTab {
            tab: TabId(1),
            to_index: 1
        }]],
        "first tab moved to the second slot, and no activation"
    );
}

#[gpui::test]
fn releasing_a_drag_where_it_started_writes_nothing(cx: &mut TestAppContext) {
    let harness = open(cx, tabs(), 1);
    let (cx, events) = (harness.cx, harness.events);
    let at = |x: f32| point(px(x), px(21.0));
    cx.simulate_mouse_down(at(108.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_move(at(130.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_move(at(108.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_up(at(108.0), MouseButton::Left, Modifiers::none());
    assert!(
        applied(&events.borrow()).is_empty(),
        "no command for a no-op drop"
    );
}
