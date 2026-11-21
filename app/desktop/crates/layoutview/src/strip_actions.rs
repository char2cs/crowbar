use std::rc::Rc;

use gpui::{App, Context, Entity};
use layout::{Command, PaneGroup};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::strip_input::state_in;
use crate::strip_keyboard::StripKeyboard;
use ideshell::OverflowEntry;
use ideshell::{StripKeys, TabAction};

impl LayoutView {
    pub(crate) fn unpin_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> TabAction {
        let (view, pane) = (cx.entity(), pane.clone());
        let unpin = LayoutEvent::Apply(Command::UnpinPane { pane });
        Rc::new(move |_, cx| tell(&view, unpin.clone(), cx))
    }

    pub(crate) fn keep_action(
        group: &PaneGroup,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> Option<TabAction> {
        if group.preview() != Some(pane) {
            return None;
        }
        let (view, pane) = (cx.entity(), pane.clone());
        let keep = LayoutEvent::Apply(Command::KeepPane { pane });
        Some(Rc::new(move |_, cx| tell(&view, keep.clone(), cx)))
    }

    pub(crate) fn add_action(
        onto: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> TabAction {
        let (view, onto) = (cx.entity(), onto.clone());
        let add = LayoutEvent::NewTab { onto };
        Rc::new(move |_, cx| tell(&view, add.clone(), cx))
    }

    pub(crate) fn keys_action(
        &self,
        group: &PaneGroup,
        cx: &mut Context<'_, Self>,
    ) -> StripKeys {
        let pinned: Vec<(PaneId, String)> = group
            .tabs()
            .iter()
            .filter(|pane| group.is_pinned(pane))
            .map(|pane| (pane.clone(), self.look_of(pane).label.to_string()))
            .collect();
        let keys = StripKeyboard::new(group.clone(), pinned);
        let view = cx.entity();
        Rc::new(move |key, _, cx| keys.press(key, &view, cx))
    }

    pub(crate) fn overflow_of(
        &self,
        group: &PaneGroup,
        cx: &mut Context<'_, Self>,
    ) -> Vec<OverflowEntry> {
        group
            .tabs()
            .iter()
            .map(|pane| OverflowEntry {
                label: self.look_of(pane).label,
                state: state_in(group, pane),
                select: Self::select_action(pane, cx),
            })
            .collect()
    }
}

fn tell(
    view: &Entity<LayoutView>,
    event: LayoutEvent,
    cx: &mut App,
) {
    view.update(cx, |_, cx| cx.emit(event));
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::Command;

    use super::*;
    use crate::test_support::{FakeSource, open_with, pane_id, stacked_layout, strip_of};
    use ideshell::TabEnd;
    use ideshell::TabState;

    #[gpui::test]
    fn the_plus_button_asks_for_a_new_tab_in_the_group_of_the_shown_tab(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), FakeSource::starting_at_first_tab());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("b"), cx))
            .expect("strip");
        cx.update(|window, cx| (strip.add)(window, cx));
        assert_eq!(
            opened.borrow().as_slice(),
            &[LayoutEvent::NewTab { onto: pane_id("c") }],
            "the shown tab is c"
        );
    }

    #[gpui::test]
    fn keys_on_the_strip_select_neighbours_and_ignore_the_rest(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), FakeSource::starting_at_first_tab());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("b"), cx))
            .expect("strip");
        cx.update(|window, cx| {
            (strip.keys)("right", window, cx);
            (strip.keys)("left", window, cx);
            (strip.keys)("x", window, cx);
        });
        assert_eq!(
            opened.borrow().as_slice(),
            &[LayoutEvent::StripFocus { pane: pane_id("b") }],
            "no right of c, b left of c, x ignored"
        );
    }

    fn pressing_delete(
        cx: &mut TestAppContext,
        layout: &layout::Layout,
    ) -> (Vec<Command>, Vec<LayoutEvent>) {
        let harness = open_with(cx, layout, FakeSource::starting_at_first_tab());
        let (view, cx) = (harness.view, harness.cx);
        let (commands, opened) = (harness.commands, harness.opened);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("b"), cx))
            .expect("strip");
        cx.update(|window, cx| (strip.keys)("delete", window, cx));
        (commands.take(), opened.take())
    }

    #[gpui::test]
    fn delete_closes_the_shown_tab(cx: &mut TestAppContext) {
        let (commands, opened) = pressing_delete(cx, &stacked_layout());
        assert_eq!(
            (commands, opened),
            (vec![Command::ClosePane { pane: pane_id("c") }], vec![]),
            "c is shown and loose"
        );
    }

    #[gpui::test]
    fn delete_on_a_pinned_shown_tab_closes_nothing_and_says_why(cx: &mut TestAppContext) {
        let mut layout = stacked_layout();
        layout
            .apply(Command::PinPane { pane: pane_id("b") })
            .unwrap();
        layout
            .apply(Command::FocusPane { pane: pane_id("b") })
            .unwrap();
        let (commands, opened) = pressing_delete(cx, &layout);
        assert_eq!(
            (commands, opened),
            (
                vec![],
                vec![LayoutEvent::Announce(
                    "Cannot close pinned tab core.terminal".into()
                )]
            ),
            "b is shown and pinned"
        );
    }

    #[gpui::test]
    fn the_more_tabs_menu_lists_every_tab_and_marks_the_shown_one(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), FakeSource::starting_at_first_tab());
        let (view, cx) = (harness.view, harness.cx);
        let entries = view.update(cx, |view, cx| {
            let strip = strip_of(view, &pane_id("b"), cx).expect("strip");
            strip.overflow
        });
        let states: Vec<_> = entries.iter().map(|entry| entry.state).collect();
        assert_eq!(
            states,
            [TabState::Idle, TabState::Active],
            "b, then c shown"
        );
    }

    #[gpui::test]
    fn a_pinned_tab_ends_in_a_pin_that_unpins_instead_of_closing(cx: &mut TestAppContext) {
        let mut layout = stacked_layout();
        layout
            .apply(Command::PinPane { pane: pane_id("c") })
            .unwrap();
        let harness = open_with(cx, &layout, FakeSource::starting_at_first_tab());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("b"), cx))
            .expect("strip");
        let ends: Vec<_> = strip.tabs.iter().map(|tab| tab.end).collect();
        cx.update(|window, cx| (strip.tabs[0].actions.close)(window, cx));
        cx.update(|window, cx| (strip.tabs[1].actions.close)(window, cx));
        assert_eq!(
            ends,
            [TabEnd::Unpin, TabEnd::Close],
            "c is pinned and in front"
        );
        assert_eq!(
            commands.borrow().as_slice(),
            &[
                Command::UnpinPane { pane: pane_id("c") },
                Command::ClosePane { pane: pane_id("b") }
            ],
            "the pin unpins, the cross closes"
        );
    }
}
