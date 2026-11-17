use std::rc::Rc;

use gpui::Context;
use model::{Command, DragSubject};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::strip_index::insertion_index;
use crate::tab_actions::TabDrop;
use crate::tab_seat::TabSeat;

impl LayoutView {
    pub(crate) fn drop_action(
        onto: &PaneId,
        before: usize,
        cx: &mut Context<'_, Self>,
    ) -> TabDrop {
        let (view, onto) = (cx.entity(), onto.clone());
        Rc::new(move |subject, _, cx| {
            cx.stop_propagation();
            Self::drop_into(&view, subject, TabSeat::at(&onto, before), cx);
        })
    }

    fn drop_into(
        view: &gpui::Entity<Self>,
        subject: &DragSubject,
        seat: TabSeat<'_>,
        cx: &mut gpui::App,
    ) {
        view.update(cx, |this, cx| this.strip_dropped(subject, seat, cx));
    }

    fn strip_dropped(
        &mut self,
        subject: &DragSubject,
        seat: TabSeat<'_>,
        cx: &mut Context<'_, Self>,
    ) {
        let (onto, before) = (seat.pane, seat.index);
        self.drop.clear();
        cx.notify();
        let DragSubject::Pane(pane) = subject else {
            return;
        };
        let Some(tab) = &self.tab else {
            return;
        };
        if !tab
            .pane(pane)
            .is_some_and(|dragged| self.source.stacks(dragged))
        {
            return;
        }
        let dragged = tab
            .group_of(onto)
            .and_then(|group| group.panes().iter().position(|member| member == pane));
        let index = Some(insertion_index(before, dragged));
        cx.emit(LayoutEvent::Apply(Command::MoveToGroup {
            pane: pane.clone(),
            onto: onto.clone(),
            index,
        }));
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::{FakeSource, open_with, pane_id, stacked_layout};

    fn stacking() -> FakeSource {
        FakeSource::starting_at_first_tab()
            .stacking(pane_id("a"))
            .stacking(pane_id("b"))
            .stacking(pane_id("c"))
    }

    #[gpui::test]
    fn dropping_on_a_strip_tab_ends_the_drag_and_releases_the_overlay(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx) = (harness.view, harness.cx);
        let subject = DragSubject::Pane(pane_id("a"));
        let drop = view.update(cx, |view, cx| {
            let centre = view.bounds.center();
            view.hover(&subject, centre, cx);
            let strip = view.strip_of(&pane_id("b"), cx).expect("strip");
            Rc::clone(&strip.tabs[0].actions.drop)
        });
        let hovering = view.read_with(cx, |view, _| view.drop.is_dragging());
        cx.update(|window, cx| drop(&subject, window, cx));
        let still = view.read_with(cx, |view, _| view.drop.is_dragging());
        assert_eq!(
            (hovering, still),
            (true, false),
            "dragging before, then not"
        );
    }
}
