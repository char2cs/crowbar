use gpui::{
    AnyElement, App, AppContext as _, Context, DismissEvent, Entity, Pixels, Point, Window,
};
use layout::{Command, TabId};
use ui::{Menu, MenuAction, MenuItem};

use crate::event::Event;
use crate::tab_bar::TabBar;

impl TabBar {
    pub(crate) fn open_menu(
        &mut self,
        (index, at): (usize, Point<Pixels>),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = self.tabs.get(index).map(|tab| tab.id) else {
            return;
        };
        let others: Vec<_> = self
            .tabs
            .iter()
            .map(|other| other.id)
            .filter(|id| *id != tab)
            .collect();
        let items = vec![
            MenuItem::Action(close_tabs("Close", vec![tab], cx.entity())),
            MenuItem::Action(close_tabs("Close Others", others, cx.entity())),
        ];
        let menu = cx.new(|cx| Menu::new(items, window, cx));
        cx.subscribe(&menu, |this, _, _: &DismissEvent, cx| {
            this.menu = None;
            cx.notify();
        })
        .detach();
        self.menu = Some(menu);
        self.menu_at = at;
        cx.notify();
    }
}

impl TabBar {
    fn close_menu(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.menu = None;
        cx.notify();
    }

    pub(crate) fn menu_layers(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Vec<AnyElement> {
        let Some(menu) = self.menu.clone() else {
            return Vec::new();
        };
        let bar = cx.entity();
        let away = ui::click_away("tab-menu-away", move |_, cx: &mut App| {
            bar.update(cx, Self::close_menu);
        });
        let popup = ui::popup_at("tab-menu", self.menu_at, move |_, _| menu.clone());
        vec![away, popup]
    }
}

fn close_tabs(
    label: &'static str,
    tabs: Vec<TabId>,
    view: Entity<TabBar>,
) -> MenuAction {
    let none = tabs.is_empty();
    let action = MenuAction::new(label, move |_, cx| {
        let commands = tabs
            .iter()
            .map(|tab| Command::CloseTab { tab: *tab })
            .collect();
        view.update(cx, |_, cx| cx.emit(Event::Apply(commands)));
    });
    if none {
        return action.disabled();
    }
    action
}

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, MouseButton, TestAppContext, point, px};

    use crate::test_support::{open, view};

    #[gpui::test]
    fn right_clicking_a_tab_opens_its_menu(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        cx.simulate_mouse_down(
            point(px(100.0), px(21.0)),
            MouseButton::Right,
            Modifiers::none(),
        );
        bar.read_with(cx, |bar, _| assert!(bar.menu.is_some(), "a context menu"));
    }

    fn right_click_at(
        cx: &mut gpui::VisualTestContext,
        x: f32,
    ) {
        cx.simulate_mouse_down(
            point(px(x), px(21.0)),
            MouseButton::Right,
            Modifiers::none(),
        );
        cx.run_until_parked();
    }

    #[gpui::test]
    fn escape_closes_the_menu(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        right_click_at(cx, 100.0);
        cx.simulate_keystrokes("escape");
        bar.read_with(cx, |bar, _| assert!(bar.menu.is_none(), "closed"));
    }

    #[gpui::test]
    fn pressing_outside_closes_the_menu(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        right_click_at(cx, 100.0);
        cx.simulate_click(point(px(700.0), px(400.0)), Modifiers::none());
        bar.read_with(cx, |bar, _| assert!(bar.menu.is_none(), "closed"));
    }

    #[gpui::test]
    fn the_menu_opens_where_the_pointer_is(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        right_click_at(cx, 100.0);
        bar.read_with(cx, |bar, _| {
            assert_eq!(bar.menu_at, point(px(100.0), px(21.0)));
        });
    }
}
