use std::rc::Rc;

use gpui::{Context, SharedString};
use layout::{CloseScope, Command, PaneGroup};
use protocol::PaneId;
use ui::{IconName, MenuAction, MenuItem};

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::path_action::PathAction;
use crate::tab_look::TabLook;

#[cfg(target_os = "macos")]
const CLOSE_HINT: &str = "\u{2318}W";
#[cfg(not(target_os = "macos"))]
const CLOSE_HINT: &str = "Ctrl+W";

impl LayoutView {
    pub(crate) fn tab_menu(
        &self,
        group: &PaneGroup,
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> Rc<[MenuItem]> {
        let view = cx.entity();
        let closing = |label: &'static str, scope: CloseScope| {
            let panes = group.closable(pane, scope);
            close_item(label, panes, &view)
        };
        let look = self.look_of(pane);
        let mut items = vec![MenuItem::Action(pin_action(group, pane, &view))];
        items.extend(rename_row(pane, &look, &view));
        items.extend([
            MenuItem::Separator,
            MenuItem::Action(self.move_action(pane, &view)),
            MenuItem::Separator,
        ]);
        items.extend(path_rows(look.path.as_deref(), &view));
        items.extend([
            MenuItem::Action(single_close(pane, &view)),
            MenuItem::Action(closing("Close Others", CloseScope::Others)),
            MenuItem::Action(closing("Close to the Right", CloseScope::ToRight)),
            MenuItem::Action(closing("Close All", CloseScope::All)),
        ]);
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
    group: &PaneGroup,
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

fn rename_row(
    pane: &PaneId,
    look: &TabLook,
    view: &gpui::Entity<LayoutView>,
) -> Option<MenuItem> {
    let event = LayoutEvent::RenameTab {
        pane: pane.clone(),
        current: look.label.to_string(),
    };
    look.renamable
        .then(|| MenuItem::Action(emitting("Rename", event, view).icon(IconName::PencilSimple)))
}

fn path_rows(
    path: Option<&str>,
    view: &gpui::Entity<LayoutView>,
) -> Vec<MenuItem> {
    let Some(path) = path else {
        return Vec::new();
    };
    let mut rows: Vec<MenuItem> = PathAction::ALL
        .into_iter()
        .map(|action| (action, LayoutEvent::tab_path(action, path)))
        .map(|(action, event)| emitting(action.label(), event, view).icon(action.icon()))
        .map(MenuItem::Action)
        .collect();
    rows.push(MenuItem::Separator);
    rows
}

fn single_close(
    pane: &PaneId,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    let command = Command::ClosePane { pane: pane.clone() };
    applying("Close", command, view)
        .icon(IconName::X)
        .shortcut(CLOSE_HINT)
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
    emitting(label, LayoutEvent::Apply(command), view)
}

fn emitting(
    label: impl Into<SharedString>,
    event: LayoutEvent,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    let view = view.clone();
    MenuAction::new(label, move |_, cx| {
        view.update(cx, |_, cx| cx.emit(event.clone()));
    })
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};
    use layout::{Command, Layout};

    use crate::layout_event::LayoutEvent;
    use crate::path_action::PathAction;
    use crate::tab_look::TabLook;
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

    struct Row {
        name: &'static str,
        keys: &'static str,
        action: PathAction,
    }

    impl Row {
        fn event(&self) -> LayoutEvent {
            LayoutEvent::TabPath {
                action: self.action,
                path: "src/main.rs".into(),
            }
        }
    }

    const PATH_ROWS: &[Row] = &[
        Row {
            name: "copy path",
            keys: "down down down enter",
            action: PathAction::CopyPath,
        },
        Row {
            name: "copy relative path",
            keys: "down down down down enter",
            action: PathAction::CopyRelativePath,
        },
        Row {
            name: "reveal",
            keys: "down down down down down enter",
            action: PathAction::Reveal,
        },
        Row {
            name: "open in terminal",
            keys: "down down down down down down enter",
            action: PathAction::OpenInTerminal,
        },
    ];

    fn file_source() -> FakeSource {
        let look = TabLook::plain("main.rs").at_path("src/main.rs");
        stacking().looking(pane_id("b"), look)
    }

    fn chosen_path_row(
        cx: &mut TestAppContext,
        keys: &str,
    ) -> Vec<LayoutEvent> {
        let harness = open_with(cx, &stacked_layout(), file_source());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(40.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        cx.simulate_keystrokes(keys);
        cx.run_until_parked();
        opened.take()
    }

    #[gpui::test]
    fn a_file_tab_offers_its_path_actions_with_the_path(cx: &mut TestAppContext) {
        for row in PATH_ROWS {
            let heard = chosen_path_row(cx, row.keys);
            assert_eq!(heard, [row.event()], "{}", row.name);
        }
    }

    #[gpui::test]
    fn a_renamable_tab_offers_rename_right_after_pin(cx: &mut TestAppContext) {
        let look = TabLook::plain("zsh").renamable();
        let source = stacking().looking(pane_id("b"), look);
        let harness = open_with(cx, &stacked_layout(), source);
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(40.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        cx.simulate_keystrokes("down down enter");
        cx.run_until_parked();
        let event = LayoutEvent::RenameTab {
            pane: pane_id("b"),
            current: "zsh".into(),
        };
        assert_eq!(opened.take(), [event], "rename");
    }

    struct Opened {
        menu: gpui::Point<gpui::Pixels>,
        tab: gpui::Bounds<gpui::Pixels>,
    }

    fn opened_at(
        cx: &mut TestAppContext,
        press: f32,
    ) -> Opened {
        let harness = open_with(cx, &stacked_layout(), stacking());
        let (view, cx) = (harness.view, harness.cx);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let at = point(size.width * 0.5 + px(press), px(20.0));
        cx.simulate_mouse_down(at, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        Opened {
            menu: cx.debug_bounds("ui-menu").unwrap_or_default().origin,
            tab: cx.debug_bounds("strip-tab-frame-b").unwrap_or_default(),
        }
    }

    #[gpui::test]
    fn the_menu_opens_four_pixels_under_the_tab_centre_whatever_part_was_pressed(
        cx: &mut TestAppContext
    ) {
        let (near, far) = (opened_at(cx, 40.0), opened_at(cx, 55.0));
        assert_eq!(near.menu, far.menu, "the pointer does not place the menu");
        let centre = near.tab.origin.x + near.tab.size.width * 0.5;
        let expected = point(centre, near.tab.bottom() + px(4.0));
        assert!(
            (near.menu - expected).x.abs() < px(0.5) && (near.menu - expected).y.abs() < px(0.5),
            "{:?} under the tab {:?}",
            near.menu,
            near.tab
        );
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
