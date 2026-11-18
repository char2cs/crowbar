//! A context menu that is open.

use gpui::{FocusHandle, Pixels, Point};

use crate::overlays::popup::menu_cursor::MenuCursor;
use crate::overlays::popup::menu_entry::MenuEntry;

#[derive(Debug)]
pub(crate) struct OpenMenu {
    pub(crate) entries: Vec<MenuEntry>,
    pub(crate) at: Point<Pixels>,
    pub(crate) cursor: MenuCursor,
    pub(crate) focus: FocusHandle,
    pub(crate) filter: bool,
}
