use crate::layout_view::LayoutView;
use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
use model::{Command, Edge};

use crate::test_support::{layout_of, open, pane_id};

fn side_by_side() -> model::Layout {
    layout_of("a", &[("b", "a", Edge::Right)])
}

#[gpui::test]
fn the_view_measures_itself_and_lays_the_panes_out_in_its_bounds(cx: &mut TestAppContext) {
    let layout = side_by_side();
    let harness = open(cx, &layout);
    let (view, cx) = (harness.view, harness.cx);
    cx.run_until_parked();
    view.read_with(cx, |view, _| {
        assert!(view.bounds.size.width > px(0.0), "measured width");
        let slots = view.tab.as_ref().expect("tab").layout(view.local());
        assert_eq!(slots.len(), 2, "two slots");
    });
}

#[gpui::test]
fn pressing_an_unfocused_pane_asks_to_focus_it(cx: &mut TestAppContext) {
    let layout = side_by_side();
    let harness = open(cx, &layout);
    let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
    view.update(cx, |view, cx| view.pane_pressed(&pane_id("a"), cx));
    cx.run_until_parked();
    assert_eq!(
        commands.borrow().as_slice(),
        &[Command::FocusPane { pane: pane_id("a") }],
        "b is focused after opening, a asks for focus"
    );
}

#[gpui::test]
fn pressing_the_focused_pane_asks_for_nothing(cx: &mut TestAppContext) {
    let layout = side_by_side();
    let harness = open(cx, &layout);
    let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
    view.update(cx, |view, cx| view.pane_pressed(&pane_id("b"), cx));
    assert!(commands.borrow().is_empty(), "already focused");
}

#[gpui::test]
fn dragging_a_sash_asks_the_model_to_resize_the_split(cx: &mut TestAppContext) {
    let layout = side_by_side();
    let harness = open(cx, &layout);
    let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
    let width = view.read_with(cx, |view, _| f32::from(view.bounds.size.width));
    let sash_x = (width - 6.0) / 2.0 + 3.0;
    let at = |x: f32| point(px(x), px(200.0));
    cx.simulate_mouse_down(at(sash_x), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_move(at(sash_x + 10.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_move(at(sash_x + 60.0), MouseButton::Left, Modifiers::none());
    cx.simulate_mouse_up(at(sash_x + 60.0), MouseButton::Left, Modifiers::none());
    let seen = commands.borrow();
    let Some(Command::ResizeSplit { first_extent, .. }) = seen.last() else {
        panic!("expected a resize, got {seen:?}");
    };
    let expected = (width - 6.0) / 2.0 + 60.0;
    assert!(
        (first_extent - expected).abs() < 1.5,
        "first extent {first_extent} vs {expected}"
    );
}

#[gpui::test]
fn the_handle_close_button_closes_that_pane(cx: &mut TestAppContext) {
    let layout = side_by_side();
    let harness = open(cx, &layout);
    let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
    view.update(cx, |_, cx| LayoutView::close(&pane_id("a"), cx));
    assert_eq!(
        commands.borrow().as_slice(),
        &[Command::ClosePane { pane: pane_id("a") }],
        "close"
    );
}

#[gpui::test]
fn detaching_asks_the_source_for_a_fresh_tab(cx: &mut TestAppContext) {
    let layout = side_by_side();
    let harness = open(cx, &layout);
    let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
    view.update(cx, |view, cx| view.detach(&pane_id("a"), cx));
    assert_eq!(
        commands.borrow().as_slice(),
        &[Command::DetachPane {
            pane: pane_id("a"),
            new_tab: model::TabId(100),
            title: "Detached".into()
        }],
        "detach into a fresh tab"
    );
}

#[gpui::test]
fn only_the_hovered_pane_of_a_split_shows_its_handle(cx: &mut TestAppContext) {
    let layout = side_by_side();
    let harness = open(cx, &layout);
    let (view, cx) = (harness.view, harness.cx);
    view.update(cx, |view, cx| view.set_hovered_pane(Some(pane_id("a")), cx));
    cx.executor()
        .advance_clock(std::time::Duration::from_millis(200));
    cx.run_until_parked();
    view.read_with(cx, |view, _| {
        let now = std::time::Instant::now() + std::time::Duration::from_millis(200);
        let motion = ui::Motion::default();
        assert_eq!(
            view.handles.opacity(&pane_id("a"), now, motion),
            1.0,
            "a shows its handle"
        );
        assert_eq!(
            view.handles.opacity(&pane_id("b"), now, motion),
            0.0,
            "b does not"
        );
        assert_eq!(
            view.rings.opacity(&pane_id("b"), now, motion),
            1.0,
            "the focused pane has the ring"
        );
    });
}

#[gpui::test]
fn a_lone_pane_has_neither_ring_nor_handle(cx: &mut TestAppContext) {
    let layout = layout_of("a", &[]);
    let harness = open(cx, &layout);
    let (view, cx) = (harness.view, harness.cx);
    view.update(cx, |view, cx| view.set_hovered_pane(Some(pane_id("a")), cx));
    view.read_with(cx, |view, _| {
        let now = std::time::Instant::now() + std::time::Duration::from_millis(200);
        let motion = ui::Motion::default();
        assert_eq!(
            view.handles.opacity(&pane_id("a"), now, motion),
            0.0,
            "no handle"
        );
        assert_eq!(
            view.rings.opacity(&pane_id("a"), now, motion),
            0.0,
            "no ring"
        );
    });
}
