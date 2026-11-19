use layout::SidebarSide;

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

impl SidebarEdge {
    /// The side the chat sits next to: a collapsed sidebar counts as the left one.
    #[must_use]
    pub fn side(self) -> SidebarSide {
        match self {
            Self::Right => SidebarSide::Right,
            Self::Closed | Self::Left => SidebarSide::Left,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_sidebar_open_on_the_right_puts_the_chat_on_the_right() {
        let cases = [
            (SidebarEdge::Closed, SidebarSide::Left),
            (SidebarEdge::Left, SidebarSide::Left),
            (SidebarEdge::Right, SidebarSide::Right),
        ];
        for (edge, side) in cases {
            assert_eq!(edge.side(), side, "{edge:?}");
        }
    }
}
