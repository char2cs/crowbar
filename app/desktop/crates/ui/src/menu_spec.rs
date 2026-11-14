//! What a dropdown opens: the rows and the minimum width.

use std::rc::Rc;

use gpui::{App, AppContext as _, Entity, Pixels, Window};

use crate::menu::Menu;
use crate::menu_item::MenuItem;

#[derive(Clone)]
pub(crate) struct MenuSpec {
    pub(crate) items: Rc<[MenuItem]>,
    pub(crate) min_width: Option<Pixels>,
}

impl MenuSpec {
    pub(crate) fn build(&self, window: &mut Window, cx: &mut App) -> Entity<Menu> {
        cx.new(|cx| {
            let menu = Menu::new(Rc::clone(&self.items), window, cx);
            let width = self.min_width.unwrap_or(Menu::DEFAULT_MIN_WIDTH);
            menu.min_width(width)
        })
    }
}

#[cfg(test)]
#[path = "menu_spec_tests.rs"]
mod tests;
