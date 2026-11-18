//! The state of a branch's workspace as the sidebar glyph shows it.

/// Lifecycle and pull-request state of a landed workspace.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WorkspaceStatus {
    /// Needs provisioning before it can be used.
    Placeholder,
    /// Locked against changes.
    Locked,
    /// A fresh branch.
    New,
    /// An open pull request with conflicts.
    PrConflicts,
    /// The branch was deleted.
    Deleted,
    /// The pull request was closed.
    PrClosed,
    /// The pull request is open.
    PrOpen,
    /// The pull request was merged.
    PrMerged,
}
