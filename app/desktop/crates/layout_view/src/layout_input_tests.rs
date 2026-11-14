use gpui::TestAppContext;
use model::Edge;

use crate::test_support::{layout_of, open, pane_id};

#[gpui::test]
fn hovering_a_pane_of_a_split_opens_an_overlay_for_its_handle_only_while_hovered(
    cx: &mut TestAppContext,
) {
    let layout = layout_of("a", &[("b", "a", Edge::Right)]);
    let harness = open(cx, &layout);
    let (view, cx) = (harness.view, harness.cx);
    view.update(cx, |view, cx| view.pane_entered(&pane_id("a"), cx));
    view.read_with(cx, |view, _| {
        assert_eq!(view.hovered, Some(pane_id("a")), "hovered");
    });
    view.update(cx, |view, cx| view.pane_left(&pane_id("a"), cx));
    view.read_with(cx, |view, _| assert_eq!(view.hovered, None, "hover ended"));
}

#[gpui::test]
fn leaving_a_pane_that_is_not_the_hovered_one_changes_nothing(cx: &mut TestAppContext) {
    let layout = layout_of("a", &[("b", "a", Edge::Right)]);
    let harness = open(cx, &layout);
    let (view, cx) = (harness.view, harness.cx);
    view.update(cx, |view, cx| view.pane_entered(&pane_id("a"), cx));
    view.update(cx, |view, cx| view.pane_left(&pane_id("b"), cx));
    view.read_with(cx, |view, _| {
        assert_eq!(view.hovered, Some(pane_id("a")), "still a");
    });
}
