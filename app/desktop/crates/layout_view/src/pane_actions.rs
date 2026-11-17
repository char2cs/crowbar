use std::rc::Rc;

use gpui::Context;
use protocol::PaneId;

use crate::handle_button::HandleAction;
use crate::layout_view::LayoutView;
use crate::pane_cell::CellAction;

impl LayoutView {
    pub(crate) fn enter_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pane_entered(&pane, cx)))
    }

    pub(crate) fn leave_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pane_left(&pane, cx)))
    }

    pub(crate) fn press_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pane_pressed(&pane, cx)))
    }

    pub(crate) fn detach_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> HandleAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.detach(&pane, cx)))
    }

    pub(crate) fn close_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> HandleAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.close_group(&pane, cx)))
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use model::Edge;

    use super::*;
    use crate::test_support::{FakeSource, layout_of, open, open_with, pane_id, stacked_layout};

    fn stacking() -> FakeSource {
        FakeSource::starting_at_first_tab()
            .stacking(pane_id("b"))
            .stacking(pane_id("c"))
    }

    #[gpui::test]
    fn the_enter_and_leave_actions_move_the_hover_to_their_pane_and_off_it(
        cx: &mut TestAppContext
    ) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        let (enter, leave) = view.update(cx, |_, cx| {
            (
                LayoutView::enter_action(&pane_id("a"), cx),
                LayoutView::leave_action(&pane_id("a"), cx),
            )
        });
        cx.update(|window, cx| enter(window, cx));
        let entered = view.read_with(cx, |view, _| view.hovered.clone());
        cx.update(|window, cx| leave(window, cx));
        let left = view.read_with(cx, |view, _| view.hovered.clone());
        assert_eq!((entered, left), (Some(pane_id("a")), None), "enter, leave");
    }

    #[gpui::test]
    fn the_press_action_focuses_the_pane_and_close_removes_it(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let harness = open(cx, &layout);
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let (press, close) = view.update(cx, |_, cx| {
            (
                LayoutView::press_action(&pane_id("a"), cx),
                LayoutView::close_action(&pane_id("a"), cx),
            )
        });
        cx.update(|window, cx| {
            press(window, cx);
            close(window, cx);
        });
        assert_eq!(commands.borrow().len(), 2, "one command per action");
    }

    #[gpui::test]
    fn closing_a_group_from_its_handle_closes_every_tab_in_one_command(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let close = view.update(cx, |_, cx| LayoutView::close_action(&pane_id("b"), cx));
        cx.update(|window, cx| close(window, cx));
        assert_eq!(
            commands.borrow().as_slice(),
            &[model::Command::ClosePanes {
                panes: vec![pane_id("b"), pane_id("c")]
            }],
            "both tabs of the group at once"
        );
    }
}
