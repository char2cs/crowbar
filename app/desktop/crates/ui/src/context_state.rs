//! The open menu of a context menu and where it was requested.

use gpui::{Entity, Pixels, Point};

use crate::menu::Menu;

#[derive(Debug, Default)]
pub(crate) struct ContextState {
    pub(crate) open: Option<Entity<Menu>>,
    pub(crate) at: Point<Pixels>,
}
