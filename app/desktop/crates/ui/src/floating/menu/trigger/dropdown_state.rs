//! The open menu of a dropdown, if any.

use gpui::{Entity, FocusHandle};

use crate::floating::menu::Menu;

#[derive(Debug)]
pub(crate) struct DropdownState {
    pub(crate) open: Option<Entity<Menu>>,
    pub(crate) trigger: FocusHandle,
}
