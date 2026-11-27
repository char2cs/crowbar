use gpui::{Pixels, Point};

use crate::floating::menu::native_menu::menu_edge::MenuEdge;
use crate::floating::menu::native_menu::native_entry::NativeEntry;

#[derive(Debug, Clone, PartialEq)]
pub struct NativeRequest {
    pub entries: Vec<NativeEntry>,
    pub at: Point<Pixels>,
    pub edge: MenuEdge,
}
