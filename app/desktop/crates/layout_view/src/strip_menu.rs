use std::rc::Rc;

use gpui::{Context, SharedString};
use model::{CloseScope, Command, Group};
use protocol::PaneId;
use ui::{IconName, MenuAction, MenuItem};

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;

impl LayoutView {
    pub(crate) fn tab_menu(
        &self,
        group: &Group,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> Rc<[MenuItem]> {
        let view = cx.entity();
        let closing = |label: &'static str, scope: CloseScope| {
            let panes = group.closable(pane, scope);
            close_item(label, panes, &view)
        };
        let items = vec![
            MenuItem::Action(pin_action(group, pane, &view)),
            MenuItem::Separator,
            MenuItem::Action(self.move_action(pane, &view)),
            MenuItem::Separator,
            MenuItem::Action(single_close(pane, &view)),
            MenuItem::Action(closing("Close Others", CloseScope::Others)),
            MenuItem::Action(closing("Close to the Right", CloseScope::ToRight)),
            MenuItem::Action(closing("Close All", CloseScope::All)),
        ];
        items.into()
    }

    fn move_action(
        &self,
        pane: &PaneId,
        view: &gpui::Entity<Self>,
    ) -> MenuAction {
        let targets = self.other_groups(pane);
        if targets.is_empty() {
            return inert("Move to Group");
        }
        let rows: Vec<MenuItem> = targets
            .into_iter()
            .map(|target| MenuItem::Action(self.move_to(pane, &target, view)))
            .collect();
        MenuAction::submenu("Move to Group", rows)
    }

    fn move_to(
        &self,
        pane: &PaneId,
        target: &PaneId,
        view: &gpui::Entity<Self>,
    ) -> MenuAction {
        let label = self.look_of(target).label;
        let command = Command::MoveToGroup {
            pane: pane.clone(),
            onto: target.clone(),
            index: None,
        };
        applying(label, command, view)
    }
    fn other_groups(
        &self,
        pane: &PaneId,
    ) -> Vec<PaneId> {
        let Some(tab) = &self.tab else {
            return Vec::new();
        };
        tab.groups(self.local())
            .into_iter()
            .filter(|slot| !slot.panes.contains(pane))
            .map(|slot| slot.active)
            .collect()
    }
}

fn pin_action(
    group: &Group,
    pane: &PaneId,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    if group.is_pinned(pane) {
        let command = Command::UnpinPane { pane: pane.clone() };
        return applying("Unpin Tab", command, view).icon(IconName::PushPinSlash);
    }
    let command = Command::PinPane { pane: pane.clone() };
    applying("Pin Tab", command, view).icon(IconName::PushPin)
}

fn single_close(
    pane: &PaneId,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    let command = Command::ClosePane { pane: pane.clone() };
    applying("Close", command, view).icon(IconName::X)
}

fn close_item(
    label: &'static str,
    panes: Vec<PaneId>,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    if panes.is_empty() {
        return inert(label);
    }
    applying(label, Command::ClosePanes { panes }, view)
}
fn inert(label: &'static str) -> MenuAction {
    MenuAction::new(label, |_, _| {}).disabled()
}
fn applying(
    label: impl Into<SharedString>,
    command: Command,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    let view = view.clone();
    MenuAction::new(label, move |_, cx| {
        view.update(cx, |_, cx| cx.emit(LayoutEvent::Apply(command.clone())));
    })
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
    use model::{Command, Layout};

    use crate::test_support::{FakeSource, open_with, pane_id, stacked_layout};

    fn stacking() -> FakeSource {
        FakeSource::starting_at_first_tab()
            .stacking(pane_id("a"))
            .stacking(pane_id("b"))
            .stacking(pane_id("c"))
    }
    fn choose(
        cx: &mut TestAppContext,
        layout: &Layout,
        keys: &str,
    ) -> Vec<Command> {
        let harness = open_with(cx, layout, stacking());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(40.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        cx.simulate_keystrokes(keys);
        cx.run_until_parked();
        commands.borrow().clone()
    }

    #[gpui::test]
    fn the_first_row_of_a_tab_menu_pins_the_tab(cx: &mut TestAppContext) {
        let chosen = choose(cx, &stacked_layout(), "down enter");
        assert_eq!(chosen, [Command::PinPane { pane: pane_id("b") }], "pin");
    }

    #[gpui::test]
    fn closing_others_closes_every_loose_tab_but_the_one_clicked(cx: &mut TestAppContext) {
        let chosen = choose(cx, &stacked_layout(), "down down down down enter");
        assert_eq!(
            chosen,
            [Command::ClosePanes {
                panes: vec![pane_id("c")]
            }],
            "others"
        );
    }

    #[gpui::test]
    fn moving_to_a_group_offers_the_other_groups_by_their_shown_tab(cx: &mut TestAppContext) {
        let chosen = choose(cx, &stacked_layout(), "down down right enter");
        assert_eq!(
            chosen,
            [Command::MoveToGroup {
                pane: pane_id("b"),
                onto: pane_id("a"),
                index: None,
            }],
            "into a's group"
        );
    }
}
