//! The context menu popup.

use gpui::{
    AnyElement, App, Context, InteractiveElement, IntoElement, MouseButton, ParentElement, Styled,
    Window, anchored, deferred, div, point, px,
};
use ui::layer;

use crate::menu_list::MenuList;
use crate::sidebar::Sidebar;

const WINDOW_MARGIN: f32 = 8.0;

impl Sidebar {
    pub(crate) fn menu_layer(
        &self,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let menu = self.menu.as_ref()?;
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
            entries: menu.entries.clone(),
            highlighted: menu.cursor.highlighted(),
            submenu: menu.cursor.open_submenu(&menu.entries),
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
        cx: &mut Context<'_, Self>,
    ) -> impl Fn(&gpui::MouseDownEvent, &mut Window, &mut App) + 'static {
        let sidebar = cx.entity();
        move |_, _, cx| sidebar.update(cx, Sidebar::close_menu)
    }
}

#[cfg(test)]
#[path = "menu_view_tests.rs"]
mod tests;
