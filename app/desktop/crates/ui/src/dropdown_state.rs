//! The open menu of a dropdown, if any.

use gpui::Entity;

use crate::menu::Menu;

/// Holds the menu entity while a dropdown is open.
#[derive(Debug, Default)]
pub(crate) struct DropdownState {
    pub(crate) open: Option<Entity<Menu>>,
}
