use std::rc::Rc;

use gpui::{Context, SharedString};
use layout::{CloseScope, Command, PaneGroup};
use protocol::PaneId;
use ui::{IconName, MenuAction, MenuItem};

use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use ideshell::PathAction;
use ideshell::TabLook;

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
    (pane, path): (&PaneId, Option<&str>),
    view: &gpui::Entity<LayoutView>,
) -> Vec<MenuItem> {
    let Some(path) = path else {
        return Vec::new();
    };
    let mut rows: Vec<MenuItem> = PathAction::offered_for(path)
        .iter()
        .map(|&action| path_row(action, path, view))
        .map(MenuItem::Action)
        .collect();
    if !PathAction::is_virtual(path) {
        let reload = LayoutEvent::ReloadTab { pane: pane.clone() };
        let reload = emitting("Reload", reload, view).icon(IconName::ArrowCounterClockwise);
        rows.push(MenuItem::Action(reload));
    }
    rows.push(MenuItem::Separator);
    rows
}

fn path_row(
    action: PathAction,
    path: &str,
    view: &gpui::Entity<LayoutView>,
) -> MenuAction {
    let event = LayoutEvent::TabPath {
        action,
        path: path.to_owned(),
    };
    emitting(action.label(), event, view).icon(action.icon())
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
    use gpui::{Bounds, Modifiers, MouseButton, Pixels, Point, TestAppContext, point, px};
    use layout::Command;

    use crate::layout_event::LayoutEvent;
    use crate::test_support::{FakeSource, open_with, pane_id, stacked_layout};
    use ideshell::PathAction;
    use ideshell::TabLook;

    type Clicked = (
        Vec<Command>,
        Vec<LayoutEvent>,
        Point<Pixels>,
        Bounds<Pixels>,
    );

    /// Right-clicks tab `b` of the stacked layout `press` pixels past the strip's middle, then
    /// types `keys` into its menu: what the menu asked for, and where it and the tab sit.
    fn right_click(
        cx: &mut TestAppContext,
        (look, press): (Option<TabLook>, f32),
        keys: &str,
    ) -> Clicked {
        let source = FakeSource::starting_at_first_tab();
        let source = match look {
            Some(look) => source.looking(pane_id("b"), look),
            None => source,
        };
        let harness = open_with(cx, &stacked_layout(), source);
        let (view, cx) = (harness.view, harness.cx);
        let size = view.read_with(cx, |view, _| view.bounds.size);
        let at = point(size.width * 0.5 + px(press), px(20.0));
        cx.simulate_mouse_down(at, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default().origin;
        let tab = cx.debug_bounds("strip-tab-frame-b").unwrap_or_default();
        cx.simulate_keystrokes(keys);
        cx.run_until_parked();
        (harness.commands.take(), harness.opened.take(), menu, tab)
    }

    #[gpui::test]
    fn each_row_of_a_tab_menu_asks_for_its_action(cx: &mut TestAppContext) {
        let file = || Some(TabLook::plain("main.rs").at_path("src/main.rs"));
        let zsh = || TabLook::plain("zsh").renamable();
        let path = |action| LayoutEvent::TabPath {
            action,
            path: "src/main.rs".into(),
        };
        let (b, c) = (pane_id("b"), pane_id("c"));
        let (pin, close) = (
            Command::PinPane { pane: b.clone() },
            Command::ClosePane { pane: b.clone() },
        );
        let others = Command::ClosePanes { panes: vec![c] };
        let reload = LayoutEvent::ReloadTab { pane: b.clone() };
        let rename = LayoutEvent::RenameTab {
            pane: b,
            current: "zsh".into(),
        };
        let cases = [
            (None, 1, vec![pin], vec![]),
            (None, 4, vec![others], vec![]),
            (file(), 2, vec![], vec![path(PathAction::CopyPath)]),
            (file(), 3, vec![], vec![path(PathAction::CopyRelativePath)]),
            (file(), 4, vec![], vec![path(PathAction::Reveal)]),
            (file(), 5, vec![], vec![path(PathAction::OpenInTerminal)]),
            (file(), 6, vec![], vec![reload]),
            (Some(zsh()), 2, vec![], vec![rename]),
            (Some(zsh().at_path("terminal://b")), 6, vec![close], vec![]),
        ];
        for (look, downs, commands, events) in cases {
            let keys = format!("{}enter", "down ".repeat(downs));
            let (heard, told, ..) = right_click(cx, (look, 74.0), &keys);
            assert_eq!((heard, told), (commands, events), "{keys}");
        }
    }

    #[gpui::test]
    fn the_menu_opens_four_pixels_under_the_tab_centre_whatever_part_was_pressed(
        cx: &mut TestAppContext
    ) {
        let (.., near, tab) = right_click(cx, (None, 74.0), "escape");
        let (.., far, _) = right_click(cx, (None, 89.0), "escape");
        assert_eq!(near, far, "the pointer does not place the menu");
        let expected = point(tab.origin.x + tab.size.width * 0.5, tab.bottom() + px(4.0));
        assert!(
            (near - expected).x.abs() < px(0.5) && (near - expected).y.abs() < px(0.5),
            "{near:?} under the tab {tab:?}"
        );
    }
}
