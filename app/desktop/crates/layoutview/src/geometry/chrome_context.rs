use layout::SidebarSide;

use crate::geometry::sidebar_edge::SidebarEdge;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct ChromeContext {
    pub sidebar: SidebarEdge,
    pub side: SidebarSide,
    pub tab_strip: bool,
}
