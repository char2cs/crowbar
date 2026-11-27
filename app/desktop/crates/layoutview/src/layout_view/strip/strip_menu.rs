use std::rc::Rc;

use gpui::{Context, SharedString};
use layout::{CloseScope, Command, PaneGroup};
use protocol::PaneId;
use ui::{MenuAction, MenuItem};

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use ideshell::PathAction;
use ideshell::TabLook;

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
        items.push(MenuItem::Separator);
        items.extend(path_rows((pane, look.path.as_deref()), &view));
        items.extend([
            MenuItem::Action(single_close(pane, &view)),
            MenuItem::Action(closing("Close Others", CloseScope::Others)),
            MenuItem::Action(closing("Close to Right", CloseScope::ToRight)),
            MenuItem::Action(closing("Close All", CloseScope::All)),
        ]);
        items.into()
    }
}

fn pin_action(
    group: &PaneGroup,
    pane: &PaneId,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    if group.is_pinned(pane) {
        let command = Command::UnpinPane { pane: pane.clone() };
        return applying("Unpin Tab", command, view);
    }
    let command = Command::PinPane { pane: pane.clone() };
    applying("Pin Tab", command, view)
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
        .then(|| MenuItem::Action(emitting("Rename", event, view)))
}

fn path_rows(
    (pane, path): (&PaneId, Option<&str>),
    view: &gpui::Entity<LayoutView>,
) -> Vec<MenuItem> {
    let Some(path) = path else {
        return Vec::new();
    };
    let mut rows: Vec<MenuItem> = PathAction::offered_for(path)
        .iter()
        .map(|action| (*action, LayoutEvent::tab_path(*action, path)))
        .map(|(action, event)| emitting(action.label(), event, view))
        .map(MenuItem::Action)
        .collect();
    if !PathAction::is_virtual(path) {
        let reload = LayoutEvent::ReloadTab { pane: pane.clone() };
        let reload = emitting("Reload", reload, view);
        rows.push(MenuItem::Action(reload));
    }
    rows.push(MenuItem::Separator);
    rows
}

fn single_close(
    pane: &PaneId,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    let command = Command::ClosePane { pane: pane.clone() };
    applying("Close", command, view)
}

fn close_item(
    label: &'static str,
    panes: Vec<PaneId>,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    applying(label, Command::ClosePanes { panes }, view)
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
    use ui::NativeEntry;
    use ui::testing::{answer_menus, entry_labelled};

    use crate::layout_event::LayoutEvent;
    use crate::test_support::{FakeSource, open_with, pane_id, stacked_layout};
    use ideshell::PathAction;
    use ideshell::TabLook;

    fn choose(
        cx: &mut TestAppContext,
        layout: &Layout,
        label: &'static str,
    ) -> Vec<Command> {
        answer_menus(cx, move |entries| entry_labelled(entries, label));
        let harness = open_with(cx, layout, FakeSource::starting_at_first_tab());
        let (view, cx, commands) = (harness.view, harness.cx, harness.commands);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(74.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(tab, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        commands.borrow().clone()
    }

    #[gpui::test]
    fn the_first_row_of_a_tab_menu_pins_the_tab(cx: &mut TestAppContext) {
        let chosen = choose(cx, &stacked_layout(), "Pin Tab");
        assert_eq!(chosen, [Command::PinPane { pane: pane_id("b") }], "pin");
    }

    const PATH_ROWS: [PathAction; 4] = [
        PathAction::CopyPath,
        PathAction::CopyRelativePath,
        PathAction::Reveal,
        PathAction::OpenInTerminal,
    ];

    fn file_source() -> FakeSource {
        let look = TabLook::plain("main.rs").at_path("src/main.rs");
        FakeSource::starting_at_first_tab().looking(pane_id("b"), look)
    }

    fn chosen_path_row(
        cx: &mut TestAppContext,
        label: &'static str,
    ) -> Vec<LayoutEvent> {
        answer_menus(cx, move |entries| entry_labelled(entries, label));
        let harness = open_with(cx, &stacked_layout(), file_source());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(74.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(tab, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        opened.take()
    }

    #[gpui::test]
    fn a_file_tab_offers_its_path_actions_with_the_path(cx: &mut TestAppContext) {
        for action in PATH_ROWS {
            let heard = chosen_path_row(cx, action.label());
            let path = "src/main.rs".into();
            let event = LayoutEvent::TabPath { action, path };
            assert_eq!(heard, [event], "{}", action.label());
        }
    }

    #[gpui::test]
    fn a_file_tab_offers_reload_after_its_path_actions(cx: &mut TestAppContext) {
        let heard = chosen_path_row(cx, "Reload");
        let reload = LayoutEvent::ReloadTab { pane: pane_id("b") };
        assert_eq!(heard, [reload]);
    }

    #[gpui::test]
    fn a_renamable_tab_offers_rename(cx: &mut TestAppContext) {
        answer_menus(cx, |entries| entry_labelled(entries, "Rename"));
        let look = TabLook::plain("zsh").renamable();
        let source = FakeSource::starting_at_first_tab().looking(pane_id("b"), look);
        let harness = open_with(cx, &stacked_layout(), source);
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(74.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(tab, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        let event = LayoutEvent::RenameTab {
            pane: pane_id("b"),
            current: "zsh".into(),
        };
        assert_eq!(opened.take(), [event], "rename");
    }

    #[gpui::test]
    fn a_terminal_tab_offers_its_path_rows_and_then_close(cx: &mut TestAppContext) {
        let popped = answer_menus(cx, |_| None);
        let look = TabLook::plain("zsh").renamable().at_path("terminal://b");
        let source = FakeSource::starting_at_first_tab().looking(pane_id("b"), look);
        let harness = open_with(cx, &stacked_layout(), source);
        let (view, cx) = (harness.view, harness.cx);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let tab = point(size.width * 0.5 + px(74.0), px(20.0));
        cx.simulate_mouse_down(tab, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(tab, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        let labels: Vec<String> = popped
            .borrow()
            .iter()
            .flat_map(|menu| menu.entries.iter())
            .filter_map(|entry| match entry {
                NativeEntry::Item { label, .. } => Some(label.to_string()),
                _ => None,
            })
            .collect();
        let close = labels.iter().position(|label| label == "Close");
        let rows = ["Pin Tab", "Rename", "Copy Path", "Copy Relative Path"];
        assert_eq!(labels.get(..rows.len()), Some(&rows.map(String::from)[..]));
        assert_eq!(
            close,
            Some(rows.len() + 1),
            "reveal, then close: {labels:?}"
        );
    }

    struct Opened {
        menu: gpui::Point<gpui::Pixels>,
        tab: gpui::Bounds<gpui::Pixels>,
    }

    fn opened_at(
        cx: &mut TestAppContext,
        press: f32,
    ) -> Opened {
        let popped = answer_menus(cx, |_| None);
        let harness = open_with(cx, &stacked_layout(), FakeSource::starting_at_first_tab());
        let (view, cx) = (harness.view, harness.cx);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let at = point(size.width * 0.5 + px(press), px(20.0));
        cx.simulate_mouse_down(at, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(at, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        let menu = popped.borrow().first().map(|menu| menu.at);
        Opened {
            menu: menu.unwrap_or_default(),
            tab: cx.debug_bounds("strip-tab-frame-b").unwrap_or_default(),
        }
    }

    #[gpui::test]
    fn the_menu_opens_four_pixels_under_the_tab_centre_whatever_part_was_pressed(
        cx: &mut TestAppContext
    ) {
        let (near, far) = (opened_at(cx, 74.0), opened_at(cx, 89.0));
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
        let chosen = choose(cx, &stacked_layout(), "Close Others");
        assert_eq!(
            chosen,
            [Command::ClosePanes {
                panes: vec![pane_id("c")]
            }],
            "others"
        );
    }
}
