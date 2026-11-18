//! Keyboard and pointer navigation of a menu.

pub(crate) mod actions;
mod cursor;

pub(crate) use actions::bindings;
pub(crate) use cursor::step;

use std::rc::Rc;

use gpui::{Context, DismissEvent, Window};

use super::{Menu, MenuItem, MenuLevel, MenuNavigation};
use crate::floating::menu::navigation::actions::{
    CloseSubmenu, Confirm, Dismiss, OpenSubmenu as OpenSubmenuAction,
};
use crate::foundation::Direction;

impl Menu {
    pub(crate) fn move_highlight(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        self.highlighted = step(&self.items, self.highlighted, direction);
        cx.notify();
    }

    pub(crate) fn choose(
        &mut self,
        index: usize,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(MenuItem::Action(action)) =
            self.items.get(index).filter(|item| item.is_selectable())
        else {
            return;
        };
        if action.submenu.is_some() {
            self.open_submenu(index, window, cx);
            return;
        }
        let on_select = Rc::clone(&action.on_select);
        cx.emit(DismissEvent);
        on_select(window, cx);
    }

    pub(crate) fn confirm(
        &mut self,
        _: &Confirm,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(index) = self.highlighted {
            self.choose(index, window, cx);
        }
    }

    pub(crate) fn highlight(
        &mut self,
        index: usize,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.highlighted == Some(index) {
            return;
        }
        self.highlighted = Some(index);
        self.hover_submenu(index, window, cx);
        cx.notify();
    }

    pub(crate) fn dismiss(
        &mut self,
        _: &Dismiss,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match self.level {
            MenuLevel::Root => cx.emit(DismissEvent),
            MenuLevel::Nested => cx.emit(MenuNavigation::Back),
        }
    }

    pub(crate) fn close_submenu(
        &mut self,
        _: &CloseSubmenu,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.level == MenuLevel::Nested {
            cx.emit(MenuNavigation::Back);
        }
    }

    pub(crate) fn open_highlighted(
        &mut self,
        _: &OpenSubmenuAction,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(index) = self.highlighted {
            self.open_submenu(index, window, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::floating::menu::tests::{Log, open_menu};

    #[gpui::test]
    fn arrow_keys_skip_disabled_rows_and_enter_runs_the_highlighted_one(cx: &mut TestAppContext) {
        let log = Log::default();
        let (_, cx, dismissed) = open_menu(cx, &log);
        cx.simulate_keystrokes("down down enter");
        assert_eq!(*log.borrow(), ["last"]);
        assert_eq!(*dismissed.borrow(), 1);
    }

    #[gpui::test]
    fn up_from_nothing_selects_the_last_row(cx: &mut TestAppContext) {
        let log = Log::default();
        let (_, cx, _) = open_menu(cx, &log);
        cx.simulate_keystrokes("up enter");
        assert_eq!(*log.borrow(), ["last"]);
    }

    #[gpui::test]
    fn enter_without_a_highlight_does_nothing(cx: &mut TestAppContext) {
        let log = Log::default();
        let (_, cx, dismissed) = open_menu(cx, &log);
        cx.simulate_keystrokes("enter");
        assert!(log.borrow().is_empty());
        assert_eq!(*dismissed.borrow(), 0);
    }
}
