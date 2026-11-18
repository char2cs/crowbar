//! A context menu that is open.

use gpui::{FocusHandle, Pixels, Point};

use crate::menu_cursor::MenuCursor;
use crate::menu_entry::MenuEntry;

#[derive(Debug)]
pub(crate) struct OpenMenu {
    pub(crate) entries: Vec<MenuEntry>,
    pub(crate) at: Point<Pixels>,
    pub(crate) cursor: MenuCursor,
    pub(crate) focus: FocusHandle,
    pub(crate) filter: bool,
}
