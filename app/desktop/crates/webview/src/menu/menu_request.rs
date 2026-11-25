use crate::menu::menu_entry::MenuEntry;

/// A native context menu to pop up over a shell window.
#[derive(Debug, Clone, PartialEq)]
pub struct MenuRequest {
    /// Its entries, top to bottom.
    pub entries: Vec<MenuEntry>,
    /// The left edge it opens at, in the window's logical pixels, as a view's bounds.
    pub x: f64,
    /// The top edge it opens at, in the window's logical pixels, as a view's bounds.
    pub y: f64,
}
