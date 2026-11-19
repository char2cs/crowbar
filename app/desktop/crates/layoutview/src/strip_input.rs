use std::rc::Rc;

use gpui::Context;
use layout::{DragSubject, PaneGroup};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::pane_strip::PaneStrip;
use crate::strip_tab::StripTab;
use crate::tab_actions::{TabAction, TabActions};
use crate::tab_end::TabEnd;
use crate::tab_look::TabLook;
use crate::tab_seat::TabSeat;
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
        group: &PaneGroup,
        cx: &mut Context<'_, Self>,
    ) -> Option<PaneStrip> {
        if group.tabs().is_empty() {
            return None;
        }
        let active = group.active_tab()?;
        let tabs = group
            .tabs()
            .iter()
            .enumerate()
            .map(|(index, pane)| self.strip_tab(group, TabSeat::at(pane, index), cx))
            .collect();
        let end = group.tabs().len();
        Some(PaneStrip {
            key: group.tabs().first()?.clone(),
            tabs,
            shown: group.tabs().iter().position(|pane| pane == active)?,
            at_end_drop: Self::drop_action(active, end, cx),
            add: Self::add_action(active, cx),
            keys: self.keys_action(group, cx),
            overflow: self.overflow_of(group, cx),
        })
    }

    fn strip_tab(
        &self,
        group: &PaneGroup,
        seat: TabSeat<'_>,
        cx: &mut Context<'_, Self>,
    ) -> StripTab {
        let pane = seat.pane;
        let look = self.look_of(pane);
        StripTab {
            pane: pane.clone(),
            look,
            state: state_in(group, pane),
            end: end_of(group, pane),
            menu: self.tab_menu(group, pane, cx),
            actions: Self::actions_of(group, seat, cx),
            ringed: false,
        }
    }

    fn actions_of(
        group: &PaneGroup,
        seat: TabSeat<'_>,
        cx: &mut Context<'_, Self>,
    ) -> TabActions {
        let remove = Self::strip_close_action(seat.pane, cx);
        TabActions {
            select: Self::select_action(seat.pane, cx),
            close: Self::end_action(group, seat.pane, &remove, cx),
            remove,
            drop: Self::drop_action(group.active_tab().unwrap_or(seat.pane), seat.index, cx),
        }
    }

    fn end_action(
        group: &PaneGroup,
        pane: &PaneId,
        remove: &TabAction,
        cx: &mut Context<'_, Self>,
    ) -> TabAction {
        if group.is_pinned(pane) {
            return Self::unpin_action(pane, cx);
        }
        Rc::clone(remove)
    }

    pub(crate) fn select_action(
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
        cx.emit(LayoutEvent::StripFocus { pane: pane.clone() });
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

    pub(crate) fn look_of(
        &self,
        pane: &PaneId,
    ) -> TabLook {
        let known = self.tab.as_ref().and_then(|tab| tab.pane(pane));
        known.map_or_else(
            || TabLook::plain(pane.as_str().to_owned()),
            |instance| self.source.tab_look(instance),
        )
    }
}

fn end_of(
    group: &PaneGroup,
    pane: &PaneId,
) -> TabEnd {
    if group.is_pinned(pane) {
        return TabEnd::Unpin;
    }
    TabEnd::Close
}

pub(crate) fn state_in(
    group: &PaneGroup,
    pane: &PaneId,
) -> TabState {
    if group.active_tab() == Some(pane) {
        return TabState::Active;
    }
    TabState::Idle
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
    use layout::{Command, DragSubject};

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
        dragged: &str,
        target: &str,
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
        let opened = harness.opened;
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
            Command::ClosePane { pane: pane_id("b") },
            Command::MoveToGroup {
                pane: pane_id("a"),
                onto: pane_id("c"),
                index: Some(0),
            },
        ];
        assert_eq!(commands.borrow().as_slice(), &expected, "close, drop");
        assert_eq!(
            opened.borrow().as_slice(),
            &[LayoutEvent::StripFocus { pane: pane_id("b") }],
            "select keeps the keyboard in the strip"
        );
    }

    #[gpui::test]
    fn dragging_a_strip_tab_over_the_layout_starts_a_drag_after_its_first_moves(
        cx: &mut TestAppContext
    ) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx) = (harness.view, harness.cx);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(40.0), px(20.0));
        let none = Modifiers::none();
        cx.simulate_mouse_down(tab, MouseButton::Left, none);
        cx.simulate_mouse_move(tab + point(px(20.0), px(2.0)), MouseButton::Left, none);
        cx.simulate_mouse_move(tab + point(px(25.0), px(10.0)), MouseButton::Left, none);
        let dragging = view.read_with(cx, |view, _| view.drop.is_dragging());
        assert!(dragging, "the first moves of a strip tab start a drag");
    }

    #[gpui::test]
    fn a_pane_joins_a_strip_only_when_both_stack_and_it_is_not_already_a_member(
        cx: &mut TestAppContext
    ) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx) = (harness.view, harness.cx);
        let joins = view.update(cx, |view, _| {
            [("b", "c"), ("a", "c"), ("c", "b")]
                .map(|(dragged, target)| joins(view, dragged, target))
        });
        assert_eq!(
            joins,
            [false, true, false],
            "b and c share a group; a does not"
        );
    }

    #[gpui::test]
    fn pressing_a_tab_of_an_unfocused_group_focuses_that_tab_and_nothing_else(
        cx: &mut TestAppContext
    ) {
        let mut layout = stacked_layout();
        layout
            .apply(Command::FocusPane { pane: pane_id("a") })
            .unwrap();
        let harness = open_with(cx, &layout, stacking());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let commands = harness.commands;
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(40.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Left, Modifiers::none());
        assert_eq!(
            opened.borrow().as_slice(),
            &[LayoutEvent::StripFocus { pane: pane_id("b") }],
            "the strip focuses the tab and keeps the keyboard"
        );
        assert!(
            commands.borrow().is_empty(),
            "the pane press behind the tab must not undo it"
        );
    }
}
