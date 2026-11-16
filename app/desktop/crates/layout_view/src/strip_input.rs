use std::rc::Rc;

use gpui::Context;
use model::{Command, DragSubject, Group};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::pane_strip::PaneStrip;
use crate::strip_index::insertion_index;
use crate::strip_tab::StripTab;
use crate::tab_actions::{TabAction, TabActions, TabDrop};
use crate::tab_look::TabLook;
use crate::tab_state::TabState;

impl LayoutView {
    pub(crate) fn strip_of(
        &self,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> Option<PaneStrip> {
        let group = self.tab.as_ref()?.group_of(pane)?;
        self.strip_for(group, cx)
    }

    fn strip_for(
        &self,
        group: &Group,
        cx: &mut Context<'_, Self>,
    ) -> Option<PaneStrip> {
        if group.panes().len() < 2 {
            return None;
        }
        let tabs = group
            .panes()
            .iter()
            .enumerate()
            .map(|(index, pane)| self.strip_tab(group, (index, pane), cx))
            .collect();
        let end = group.panes().len();
        Some(PaneStrip {
            tabs,
            at_end_drop: Self::drop_action(group.active(), end, cx),
        })
    }

    fn strip_tab(
        &self,
        group: &Group,
        (index, pane): (usize, &PaneId),
        cx: &mut Context<'_, Self>,
    ) -> StripTab {
        let look = self.look_of(pane);
        StripTab {
            pane: pane.clone(),
            label: look.label,
            icon: look.icon,
            state: state_in(group, pane),
            actions: Self::actions_of(group, (index, pane), cx),
        }
    }

    fn actions_of(
        group: &Group,
        (index, pane): (usize, &PaneId),
        cx: &mut Context<'_, Self>,
    ) -> TabActions {
        TabActions {
            select: Self::select_action(pane, cx),
            close: Self::strip_close_action(pane, cx),
            drop: Self::drop_action(group.active(), index, cx),
        }
    }

    fn select_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> TabAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |_, cx| Self::emit_focus(&pane, cx)))
    }

    fn strip_close_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> TabAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |_, cx| Self::close(&pane, cx)))
    }

    fn emit_focus(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) {
        cx.emit(LayoutEvent::Apply(Command::FocusPane {
            pane: pane.clone(),
        }));
    }

    pub(crate) fn joins_strip(
        &self,
        subject: &DragSubject,
        target: &PaneId,
    ) -> bool {
        let (DragSubject::Pane(dragged), Some(tab)) = (subject, &self.tab) else {
            return false;
        };
        let own_group = tab
            .group_of(dragged)
            .is_some_and(|group| group.contains(target));
        let stacks = |pane: &PaneId| {
            tab.pane(pane)
                .is_some_and(|found| self.source.stacks(found))
        };
        !own_group && stacks(dragged) && stacks(target)
    }

    fn look_of(
        &self,
        pane: &PaneId,
    ) -> TabLook {
        let known = self.tab.as_ref().and_then(|tab| tab.pane(pane));
        known.map_or_else(
            || TabLook::plain(pane.as_str().to_owned()),
            |instance| self.source.tab_look(instance),
        )
    }

    fn drop_action(
        onto: &PaneId,
        before: usize,
        cx: &mut Context<'_, Self>,
    ) -> TabDrop {
        let (view, onto) = (cx.entity(), onto.clone());
        Rc::new(move |subject, _, cx| {
            cx.stop_propagation();
            Self::drop_into(&view, subject, (&onto, before), cx);
        })
    }

    fn drop_into(
        view: &gpui::Entity<Self>,
        subject: &DragSubject,
        seat: (&PaneId, usize),
        cx: &mut gpui::App,
    ) {
        view.update(cx, |this, cx| this.strip_dropped(subject, seat, cx));
    }

    fn strip_dropped(
        &mut self,
        subject: &DragSubject,
        (onto, before): (&PaneId, usize),
        cx: &mut Context<'_, Self>,
    ) {
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

fn state_in(
    group: &Group,
    pane: &PaneId,
) -> TabState {
    if pane == group.active() {
        return TabState::Active;
    }
    TabState::Idle
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use model::{Command, DragSubject};

    use super::*;
    use crate::test_support::{FakeSource, open_with, pane_id, stacked_layout};

    fn stacking() -> FakeSource {
        FakeSource::starting_at_first_tab()
            .stacking(pane_id("a"))
            .stacking(pane_id("b"))
            .stacking(pane_id("c"))
    }

    fn joins(
        view: &LayoutView,
        (dragged, target): (&str, &str),
    ) -> bool {
        view.joins_strip(&DragSubject::Pane(pane_id(dragged)), &pane_id(target))
    }

    #[gpui::test]
    fn only_a_group_of_two_or_more_has_a_strip_with_a_tab_per_member(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx) = (harness.view, harness.cx);
        let counts = view.update(cx, |view, cx| {
            ["a", "b", "c"].map(|pane| view.strip_of(&pane_id(pane), cx).map(|s| s.tabs.len()))
        });
        assert_eq!(counts, [None, Some(2), Some(2)], "strips of a, b, c");
    }

    #[gpui::test]
    fn the_tab_actions_focus_close_and_drop_before_their_index(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let strip = view
            .update(cx, |view, cx| view.strip_of(&pane_id("b"), cx))
            .expect("strip");
        let first = &strip.tabs[0].actions;
        cx.update(|window, cx| {
            (first.select)(window, cx);
            (first.close)(window, cx);
            (first.drop)(&DragSubject::Pane(pane_id("a")), window, cx);
        });
        let expected = [
            Command::FocusPane { pane: pane_id("b") },
            Command::ClosePane { pane: pane_id("b") },
            Command::MoveToGroup {
                pane: pane_id("a"),
                onto: pane_id("c"),
                index: Some(0),
            },
        ];
        assert_eq!(
            commands.borrow().as_slice(),
            &expected,
            "select, close, drop"
        );
    }

    #[gpui::test]
    fn a_pane_joins_a_strip_only_when_both_stack_and_it_is_not_already_a_member(
        cx: &mut TestAppContext
    ) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx) = (harness.view, harness.cx);
        let joins = view.update(cx, |view, _| {
            [("b", "c"), ("a", "c"), ("c", "b")].map(|pair| joins(view, pair))
        });
        assert_eq!(
            joins,
            [false, true, false],
            "b and c share a group; a does not"
        );
    }
}
