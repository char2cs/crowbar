//! The context menu popup.

mod check;
mod filter_menu;
mod item_look;
mod menu_cursor;
mod menu_entry;
mod menu_list;
mod menu_ops;
mod menu_step;
mod open_menu;
pub(crate) mod tone;

pub(crate) use filter_menu::filter_menu;
pub(crate) use menu_entry::MenuEntry;
pub(crate) use open_menu::OpenMenu;

use gpui::{
    AnyElement, App, Context, InteractiveElement, IntoElement, MouseButton, ParentElement, Styled,
    Window, anchored, deferred, div, point, px,
};
use ui::layer;

use crate::overlays::popup::menu_list::MenuList;
use crate::sidebar::Sidebar;

const WINDOW_MARGIN: f32 = 8.0;

impl Sidebar {
    pub(crate) fn menu_layer(
        &self,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let menu = self.menu.as_ref()?;
        let entries = self.menu_entries();
        let viewport = window.viewport_size();
        let (close_left, close_right) = (Self::closer(cx), Self::closer(cx));
        let away = div()
            .occlude()
            .w(viewport.width)
            .h(viewport.height)
            .on_mouse_down(MouseButton::Left, close_left)
            .on_mouse_down(MouseButton::Right, close_right);
        let list = MenuList {
            sidebar: cx.entity(),
            highlighted: menu.cursor.highlighted(),
            submenu: menu.cursor.open_submenu(&entries),
            entries,
            focus: Some(menu.focus.clone()),
        };
        let popup = anchored()
            .position(menu.at)
            .snap_to_window_with_margin(px(WINDOW_MARGIN))
            .child(list);
        let scrim = anchored().position(point(px(0.0), px(0.0))).child(away);
        Some(
            div()
                .child(deferred(scrim).with_priority(layer::MENU - 1))
                .child(deferred(popup).with_priority(layer::MENU))
                .into_any_element(),
        )
    }

    fn closer(
        cx: &mut Context<'_, Self>
    ) -> impl Fn(&gpui::MouseDownEvent, &mut Window, &mut App) + 'static {
        let sidebar = cx.entity();
        move |_, _, cx| sidebar.update(cx, Sidebar::close_menu)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Intent;

    use crate::test_support::prelude::*;

    use crate::overlays::popup::menu_entry::MenuEntry;
    use ui::IconName;

    #[gpui::test]
    fn an_open_menu_renders_over_the_sidebar_and_closes_on_escape(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let entries = vec![MenuEntry::action(
            "Keep",
            IconName::Check,
            Intent::Keep(RowId::from("chat")),
        )];
        sidebar.update_in(cx, |this, window, cx| {
            this.open_menu((entries, point(px(50.0), px(200.0))), window, cx);
        });
        cx.run_until_parked();
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_some()));
        sidebar.update_in(cx, |this, window, cx| this.menu_key("escape", window, cx));
        cx.run_until_parked();
        assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
    }
}
