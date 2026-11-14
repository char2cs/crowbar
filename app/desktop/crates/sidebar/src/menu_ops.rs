//! Opening, navigating and closing the context menu.

use gpui::{Context, Pixels, Point, Window};

use crate::intent::Intent;
use crate::menu_cursor::MenuCursor;
use crate::menu_entry::MenuEntry;
use crate::menu_step::MenuStep;
use crate::open_menu::OpenMenu;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn open_menu(
        &mut self,
        (entries, at): (Vec<MenuEntry>, Point<Pixels>),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if entries.is_empty() {
            return;
        }
        let focus = cx.focus_handle();
        window.focus(&focus);
        let cursor = MenuCursor::default();
        self.menu = Some(OpenMenu {
            entries,
            at,
            cursor,
            focus,
        });
        cx.notify();
    }

    pub(crate) fn close_menu(&mut self, cx: &mut Context<'_, Self>) {
        self.menu = None;
        cx.notify();
    }

    pub(crate) fn highlight(&mut self, index: Option<usize>, cx: &mut Context<'_, Self>) {
        let Some(menu) = self.menu.as_mut() else {
            return;
        };
        menu.cursor.highlight(index);
        cx.notify();
    }

    pub(crate) fn menu_key(&mut self, key: &str, window: &mut Window, cx: &mut Context<'_, Self>) {
        match key {
            "down" => self.step_menu(MenuStep::Next, cx),
            "up" => self.step_menu(MenuStep::Previous, cx),
            "enter" => self.choose_highlighted(window, cx),
            "escape" => self.close_menu(cx),
            _ => {},
        }
    }

    fn step_menu(&mut self, step: MenuStep, cx: &mut Context<'_, Self>) {
        let Some(menu) = self.menu.as_mut() else {
            return;
        };
        menu.cursor.step(step, &menu.entries);
        cx.notify();
    }

    pub(crate) fn choose(
        &mut self,
        intent: Intent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.menu = None;
        self.apply(intent, window, cx);
    }

    fn choose_highlighted(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) {
        let chosen = self
            .menu
            .as_ref()
            .and_then(|menu| menu.cursor.chosen(&menu.entries));
        if let Some(intent) = chosen {
            self.choose(intent, window, cx);
        }
    }
}

#[cfg(test)]
#[path = "menu_ops_tests.rs"]
mod tests;
