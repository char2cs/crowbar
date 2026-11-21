//! Opening, closing and placing the nested menu of a submenu row.

pub(crate) mod level;
pub(crate) mod navigation;
pub(crate) mod open_submenu;

use gpui::{
    AnyElement, AppContext as _, Context, DismissEvent, IntoElement, ParentElement, Styled, Window,
    div,
};

use crate::floating::menu::Menu;
use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::navigation::step;
use crate::floating::menu::nested::navigation::Navigation;
use crate::floating::menu::nested::open_submenu::OpenSubmenu;
use crate::floating::popup;
use crate::foundation::Direction;

impl Menu {
    pub(crate) fn open_submenu(
        &mut self,
        index: usize,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(items) = self.items.get(index).and_then(MenuItem::submenu_items) else {
            return;
        };
        if self
            .submenu
            .as_ref()
            .is_some_and(|open| open.index == index)
        {
            return;
        }
        let menu = cx.new(|cx| Menu::nested(items, window, cx));
        menu.update(cx, |nested, cx| {
            nested.highlighted = step(&nested.items, None, Direction::Forward);
            cx.notify();
        });
        let dismissed = cx.subscribe(&menu, |_, _, _: &DismissEvent, cx| cx.emit(DismissEvent));
        let returned = cx.subscribe_in(&menu, window, |this, _, _: &Navigation, window, cx| {
            this.submenu = None;
            window.focus(&this.focus_handle, cx);
            cx.notify();
        });
        self.submenu = Some(OpenSubmenu {
            index,
            menu,
            _subscriptions: [dismissed, returned],
        });
        cx.notify();
    }

    pub(crate) fn hover_submenu(
        &mut self,
        index: usize,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let opens = self
            .items
            .get(index)
            .is_some_and(|item| item.submenu_items().is_some());
        if opens {
            self.open_submenu(index, window, cx);
            return;
        }
        self.submenu = None;
    }

    pub(crate) fn with_submenu(
        &self,
        index: usize,
        row: AnyElement,
    ) -> AnyElement {
        let Some(open) = self.submenu.as_ref().filter(|open| open.index == index) else {
            return row;
        };
        div()
            .relative()
            .child(row)
            .child(popup::beside(open.menu.clone()))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Entity, TestAppContext, VisualTestContext};

    use super::*;
    use crate::floating::menu::menu_item::MenuAction;
    use crate::foundation::Motion;
    use crate::foundation::Theme;

    fn items() -> Vec<MenuItem> {
        let nested = vec![MenuItem::Action(MenuAction::new("Main", |_, _| {}))];
        vec![
            MenuItem::Action(MenuAction::submenu("Move", nested)),
            MenuItem::Action(MenuAction::new("Plain", |_, _| {})),
        ]
    }

    fn open_index(
        menu: &Entity<Menu>,
        cx: &mut VisualTestContext,
    ) -> Option<usize> {
        menu.read_with(cx, |menu, _| menu.submenu.as_ref().map(|open| open.index))
    }

    fn hover(
        menu: &Entity<Menu>,
        index: usize,
        cx: &mut VisualTestContext,
    ) {
        menu.update_in(cx, |menu, window, cx| menu.hover_submenu(index, window, cx));
    }

    #[gpui::test]
    fn hovering_a_submenu_row_opens_it_and_a_plain_row_closes_it(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let (menu, cx) = cx.add_window_view(|window, cx| Menu::new(items(), window, cx));
        hover(&menu, 0, cx);
        assert_eq!(open_index(&menu, cx), Some(0));
        hover(&menu, 1, cx);
        assert_eq!(open_index(&menu, cx), None);
    }
}
