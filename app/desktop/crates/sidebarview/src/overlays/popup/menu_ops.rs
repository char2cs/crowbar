//! Opening, navigating and closing the context menu.

use gpui::{Context, Pixels, Point, Window};
use sidebar::Intent;

use crate::overlays::popup::filter_menu::filter_menu;
use crate::overlays::popup::menu_cursor::MenuCursor;
use crate::overlays::popup::menu_entry::MenuEntry;
use crate::overlays::popup::menu_step::MenuStep;
use crate::overlays::popup::open_menu::OpenMenu;
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
        window.focus(&focus, cx);
        // PLATFORM: gpui gives focus to the pressed ancestor after the click handler ran; claim it back.
        let later = focus.clone();
        window.defer(cx, move |window, cx| window.focus(&later, cx));
        let cursor = MenuCursor::default();
        self.menu = Some(OpenMenu {
            entries,
            at,
            cursor,
            focus,
            filter: false,
        });
        cx.notify();
    }

    pub(crate) fn close_menu(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.menu = None;
        cx.notify();
    }

    pub(crate) fn highlight(
        &mut self,
        index: Option<usize>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(menu) = self.menu.as_mut() else {
            return;
        };
        menu.cursor.highlight(index);
        cx.notify();
    }

    pub(crate) fn menu_key(
        &mut self,
        key: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match key {
            "down" => self.step_menu(MenuStep::Next, cx),
            "up" => self.step_menu(MenuStep::Previous, cx),
            "enter" => self.choose_highlighted(window, cx),
            "escape" => self.close_menu(cx),
            _ => {},
        }
    }

    fn step_menu(
        &mut self,
        step: MenuStep,
        cx: &mut Context<'_, Self>,
    ) {
        let entries = self.menu_entries();
        let Some(menu) = self.menu.as_mut() else {
            return;
        };
        menu.cursor.step(step, &entries);
        cx.notify();
    }

    pub(crate) fn choose(
        &mut self,
        intent: Intent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.menu.as_ref().is_some_and(|menu| menu.filter) {
            self.menu = None;
        }
        self.apply(intent, window, cx);
    }

    pub(crate) fn menu_entries(&self) -> Vec<MenuEntry> {
        let Some(menu) = self.menu.as_ref() else {
            return Vec::new();
        };
        if menu.filter {
            return filter_menu(&self.data.file_tree_view());
        }
        menu.entries.clone()
    }

    fn choose_highlighted(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let entries = self.menu_entries();
        let chosen = self
            .menu
            .as_ref()
            .and_then(|menu| menu.cursor.chosen(&entries));
        if let Some(intent) = chosen {
            self.choose(intent, window, cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    use crate::overlays::popup::menu_entry::MenuEntry;
    use ui::IconName;

    fn entries() -> Vec<MenuEntry> {
        vec![
            MenuEntry::action(
                "Rename",
                IconName::PencilSimpleLine,
                Intent::StartRename(RowId::from("chat")),
            ),
            MenuEntry::Separator,
            MenuEntry::action("Keep", IconName::Check, Intent::Keep(RowId::from("chat"))),
        ]
    }

    fn show(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) {
        let at = point(px(10.0), px(10.0));
        sidebar.update_in(cx, |this, window, cx| {
            this.open_menu((entries(), at), window, cx);
        });
    }

    #[gpui::test]
    fn an_empty_menu_does_not_open(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let at = point(px(0.0), px(0.0));
        sidebar.update_in(cx, |this, window, cx| {
            this.open_menu((Vec::new(), at), window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
    }

    #[gpui::test]
    fn escape_closes_the_menu(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        show(&sidebar, cx);
        sidebar.update_in(cx, |this, window, cx| this.menu_key("escape", window, cx));
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
    }

    #[gpui::test]
    fn arrows_skip_separators_and_enter_applies_the_entry(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        show(&sidebar, cx);
        sidebar.update_in(cx, |this, window, cx| {
            this.menu_key("down", window, cx);
            this.menu_key("down", window, cx);
            this.menu_key("enter", window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
    }

    #[gpui::test]
    fn choosing_an_entry_closes_the_menu_and_applies_its_intent(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        show(&sidebar, cx);
        sidebar.update_in(cx, |this, window, cx| {
            this.choose(Intent::StartRename(RowId::from("chat")), window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none() && this.inline.is_some()));
    }

    #[gpui::test]
    fn highlight_without_a_menu_is_ignored(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.highlight(Some(1), cx));
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
    }
}
