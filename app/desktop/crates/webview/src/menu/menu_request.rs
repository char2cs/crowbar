use crate::menu::menu_edge::MenuEdge;
use crate::menu::menu_entry::MenuEntry;

#[derive(Debug, Clone, PartialEq)]
pub struct MenuRequest {
    pub entries: Vec<MenuEntry>,
    pub x: f64,
    pub y: f64,
    pub edge: MenuEdge,
}
