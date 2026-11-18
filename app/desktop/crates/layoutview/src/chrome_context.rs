use crate::sidebar_edge::SidebarEdge;

/// What surrounds the pane layout and so decides how its outer panes are framed.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct ChromeContext {
    /// Where the open sidebar sits.
    pub sidebar: SidebarEdge,
    /// Whether the tab strip is drawn above the layout; without it the top panes sit in the
    /// window's title area and keep a taller top margin.
    pub tab_strip: bool,
}
