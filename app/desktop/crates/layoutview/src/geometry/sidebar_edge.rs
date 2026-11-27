#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum SidebarEdge {
    #[default]
    Closed,
    Left,
    Right,
}
