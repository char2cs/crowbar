use std::rc::Rc;

use gpui::{App, Context, Entity};
use model::{Command, Group};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::overflow_entry::OverflowEntry;
use crate::strip_input::state_in;
use crate::strip_key::strip_command;
use crate::tab_actions::{StripKeys, TabAction};

impl LayoutView {
    pub(crate) fn unpin_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> TabAction {
        let (view, pane) = (cx.entity(), pane.clone());
        let unpin = LayoutEvent::Apply(Command::UnpinPane { pane });
        Rc::new(move |_, cx| tell(&view, unpin.clone(), cx))
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
        group: &Group,
        cx: &mut Context<'_, Self>,
    ) -> StripKeys {
        let (view, group) = (cx.entity(), group.clone());
        Rc::new(move |key, _, cx| {
            let command = strip_command(&group, group.active(), key);
            tell_command(&view, command, cx);
        })
    }

    pub(crate) fn overflow_of(
        &self,
        group: &Group,
        cx: &mut Context<'_, Self>,
    ) -> Vec<OverflowEntry> {
        group
            .panes()
            .iter()
            .map(|pane| OverflowEntry {
                label: self.look_of(pane).label,
                state: state_in(group, pane),
                select: Self::select_action(pane, cx),
            })
            .collect()
    }
}

fn tell_command(
    view: &Entity<LayoutView>,
    command: Option<Command>,
    cx: &mut App,
) {
    let Some(command) = command else {
        return;
    };
    tell(view, LayoutEvent::Apply(command), cx);
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
    use model::Command;

    use super::*;
    use crate::tab_end::TabEnd;
    use crate::tab_state::TabState;
    use crate::test_support::{FakeSource, open_with, pane_id, stacked_layout};

    fn stacking() -> FakeSource {
        FakeSource::starting_at_first_tab()
            .stacking(pane_id("a"))
            .stacking(pane_id("b"))
            .stacking(pane_id("c"))
    }

    #[gpui::test]
    fn the_plus_button_asks_for_a_new_tab_in_the_group_of_the_shown_tab(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let strip = view
            .update(cx, |view, cx| view.strip_of(&pane_id("b"), cx))
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
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let strip = view
            .update(cx, |view, cx| view.strip_of(&pane_id("b"), cx))
            .expect("strip");
        cx.update(|window, cx| {
            (strip.keys)("right", window, cx);
            (strip.keys)("left", window, cx);
            (strip.keys)("x", window, cx);
        });
        assert_eq!(
            commands.borrow().as_slice(),
            &[Command::FocusPane { pane: pane_id("b") }],
            "no right of c, b left of c, x ignored"
        );
    }

    #[gpui::test]
    fn the_more_tabs_menu_lists_every_tab_and_marks_the_shown_one(cx: &mut TestAppContext) {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx) = (harness.view, harness.cx);
        let entries = view.update(cx, |view, cx| {
            let strip = view.strip_of(&pane_id("b"), cx).expect("strip");
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
        let harness = open_with(cx, &layout, stacking());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let strip = view
            .update(cx, |view, cx| view.strip_of(&pane_id("b"), cx))
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
