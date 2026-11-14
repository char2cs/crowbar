use gpui::{AppContext as _, Context, DismissEvent, Entity, Window};
use model::{Command, TabId};
use ui::{Menu, MenuAction, MenuItem};

use crate::tab_bar::TabBar;
use crate::tab_bar_event::TabBarEvent;

impl TabBar {
    pub(crate) fn open_menu(
        &mut self,
        index: usize,
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
        cx.notify();
    }
}

fn close_tabs(label: &'static str, tabs: Vec<TabId>, view: Entity<TabBar>) -> MenuAction {
    let none = tabs.is_empty();
    let action = MenuAction::new(label, move |_, cx| {
        let commands = tabs
            .iter()
            .map(|tab| Command::CloseTab { tab: *tab })
            .collect();
        view.update(cx, |_, cx| cx.emit(TabBarEvent::Apply(commands)));
    });
    if none { action.disabled() } else { action }
}

#[cfg(test)]
#[path = "tab_bar_menu_tests.rs"]
mod tests;
