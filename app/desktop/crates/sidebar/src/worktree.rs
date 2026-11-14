//! Whether a row owns a worktree.

/// Rows that own a worktree can fork; branches without one are plain containers.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Worktree {
    /// Shares its parent's checkout.
    #[default]
    Shared,
    /// Has a worktree of its own.
    Owned,
}
