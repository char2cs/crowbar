//! What the focused pane tells the sidebar about the workspace it shows.

/// The focused workspace context.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Focus {
    /// No workspace is focused.
    #[default]
    Nothing,
    /// A workspace without a repository.
    Workspace,
    /// A workspace of a repository.
    Repository,
}
