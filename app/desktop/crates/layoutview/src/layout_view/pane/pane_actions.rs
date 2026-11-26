use std::rc::Rc;

use gpui::Context;
use protocol::PaneId;

use crate::layout_view::LayoutView;
use crate::layout_view::hover_source::HoverSource;
use crate::layout_view::pane::handle_button::HandleAction;
use crate::layout_view::pane::pane_cell::CellAction;

impl LayoutView {
    pub(crate) fn enter_action(
        (source, pane): (HoverSource, &PaneId),
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pointer_entered((source, &pane), cx)))
    }

    pub(crate) fn leave_action(
        (source, pane): (HoverSource, &PaneId),
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pointer_left((source, &pane), cx)))
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
    use layout::{Command, Edge};

    use super::*;
    use crate::test_support::{layout_of, open, pane_id, stacked_layout};

    #[gpui::test]
    fn the_close_action_closes_its_pane_or_every_tab_of_its_group_at_once(cx: &mut TestAppContext) {
        let group = Command::ClosePanes {
            panes: vec![pane_id("b"), pane_id("c")],
        };
        let cases = [
            (
                layout_of("a", &[("b", "a", Edge::Right)]),
                "a",
                Command::ClosePane { pane: pane_id("a") },
            ),
            (stacked_layout(), "b", group),
        ];
        for (layout, pane, expected) in cases {
            let harness = open(cx, &layout);
            let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
            let close = view.update(cx, |_, cx| LayoutView::close_action(&pane_id(pane), cx));
            cx.update(|window, cx| close(window, cx));
            assert_eq!(*commands.borrow(), [expected], "close from {pane}'s handle");
        }
    }
}
