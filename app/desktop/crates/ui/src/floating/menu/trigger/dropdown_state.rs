//! The open menu of a dropdown, if any.

use gpui::Entity;

use crate::floating::menu::Menu;

#[derive(Debug, Default)]
pub(crate) struct DropdownState {
    pub(crate) open: Option<Entity<Menu>>,
}
