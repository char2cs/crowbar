//! What a native menu shows and where.

use gpui::{Pixels, Point};

use crate::floating::menu::native_menu::menu_edge::MenuEdge;
use crate::floating::menu::native_menu::native_entry::NativeEntry;

/// A native menu to pop up over a window.
#[derive(Debug, Clone, PartialEq)]
pub struct NativeRequest {
    /// Its entries, top to bottom.
    pub entries: Vec<NativeEntry>,
    /// The window point its top `edge` corner opens at.
    pub at: Point<Pixels>,
    /// Which of its edges lines up with `at`.
    pub edge: MenuEdge,
}
