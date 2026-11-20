/// The window edge the open sidebar occupies. A pane touching that edge has a neighbour, so it
/// keeps its border and rounded corner there.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum SidebarEdge {
    /// The sidebar is collapsed.
    #[default]
    Closed,
    /// Open on the left.
    Left,
    /// Open on the right.
    Right,
}
