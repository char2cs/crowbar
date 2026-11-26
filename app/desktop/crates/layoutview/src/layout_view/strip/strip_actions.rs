use std::rc::Rc;

use gpui::{App, Context, Entity};
use layout::{Command, PaneGroup};
use protocol::PaneId;

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::layout_view::strip::strip_input::state_in;
use crate::layout_view::strip::strip_keyboard::StripKeyboard;
use ideshell::OverflowEntry;
use ideshell::{StripKeys, TabAction, TabMove};

impl LayoutView {
    pub(crate) fn emitting(
        event: LayoutEvent,
        cx: &mut Context<'_, Self>,
    ) -> TabAction {
        let view = cx.entity();
        Rc::new(move |_, cx| tell(&view, event.clone(), cx))
    }

    pub(crate) fn keep_action(
        group: &PaneGroup,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> Option<TabAction> {
        let keep = LayoutEvent::Apply(Command::KeepPane { pane: pane.clone() });
        (group.preview() == Some(pane)).then(|| Self::emitting(keep, cx))
    }

    pub(crate) fn reorder_action(cx: &mut Context<'_, Self>) -> TabMove {
        let view = cx.entity();
        Rc::new(move |pane, index, _, cx| {
            tell(&view, LayoutEvent::Apply(reordering(pane, index)), cx);
        })
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

fn reordering(
    pane: &PaneId,
    index: usize,
) -> Command {
    Command::MoveToGroup {
        pane: pane.clone(),
        onto: pane.clone(),
        index: Some(index),
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
    use layout::{Command, Layout};

    use super::*;
    use crate::test_support::{open, pane_id, stacked_layout, strip_of};
    use ideshell::TabEnd;
    use ideshell::TabState;

    fn pinned(
        mut layout: Layout,
        pane: &str,
    ) -> Layout {
        let pane = pane_id(pane);
        layout.apply(Command::PinPane { pane }).expect("pinned");
        layout
    }

    fn pressing(
        cx: &mut TestAppContext,
        layout: &Layout,
        keys: &[&str],
    ) -> (Vec<Command>, Vec<LayoutEvent>) {
        let harness = open(cx, layout);
        let (view, cx) = (harness.view, harness.cx);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("b"), cx))
            .expect("strip");
        for key in keys {
            cx.update(|window, cx| (strip.keys)(key, window, cx));
        }
        (harness.commands.take(), harness.opened.take())
    }

    #[gpui::test]
    fn keys_on_the_strip_select_neighbours_close_the_shown_tab_and_spare_a_pinned_one(
        cx: &mut TestAppContext
    ) {
        let mut pinned_b = pinned(stacked_layout(), "b");
        let focus_b = Command::FocusPane { pane: pane_id("b") };
        pinned_b.apply(focus_b).expect("b shown");
        let announce = LayoutEvent::Announce("Cannot close pinned tab core.terminal".into());
        let close_c = Command::ClosePane { pane: pane_id("c") };
        let cases = [
            (
                stacked_layout(),
                &["right", "left", "x"][..],
                vec![],
                vec![LayoutEvent::StripFocus { pane: pane_id("b") }],
            ),
            (stacked_layout(), &["delete"][..], vec![close_c], vec![]),
            (pinned_b, &["delete"][..], vec![], vec![announce]),
        ];
        for (layout, keys, commands, events) in cases {
            let heard = pressing(cx, &layout, keys);
            assert_eq!(
                heard,
                (commands, events),
                "{keys:?}: no right of c, x ignored"
            );
        }
    }

    #[gpui::test]
    fn the_plus_adds_by_the_shown_tab_and_the_more_tabs_menu_marks_it(cx: &mut TestAppContext) {
        let harness = open(cx, &stacked_layout());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let strip = view
            .update(cx, |view, cx| strip_of(view, &pane_id("b"), cx))
            .expect("strip");
        cx.update(|window, cx| (strip.add)(window, cx));
        let states: Vec<_> = strip.overflow.iter().map(|entry| entry.state).collect();
        assert_eq!(
            states,
            [TabState::Idle, TabState::Active],
            "b, then c shown"
        );
        assert_eq!(
            *opened.borrow(),
            [LayoutEvent::NewTab { onto: pane_id("c") }]
        );
    }

    #[gpui::test]
    fn a_pinned_tab_ends_in_a_pin_that_unpins_instead_of_closing(cx: &mut TestAppContext) {
        let harness = open(cx, &pinned(stacked_layout(), "c"));
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
        let expected = [
            Command::UnpinPane { pane: pane_id("c") },
            Command::ClosePane { pane: pane_id("b") },
        ];
        assert_eq!(
            *commands.borrow(),
            expected,
            "the pin unpins, the cross closes"
        );
    }
}
