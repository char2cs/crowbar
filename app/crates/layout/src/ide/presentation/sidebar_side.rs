/// The window side the sidebar sits on; side by side, the chat is the view next to it.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum SidebarSide {
    /// The sidebar is on the left.
    #[default]
    Left,
    /// The sidebar is on the right.
    Right,
}
