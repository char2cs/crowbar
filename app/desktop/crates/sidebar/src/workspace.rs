//! The workspace a branch row has landed.

use crate::workspace_status::WorkspaceStatus;

/// What the daemon reports about a branch's checkout.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Workspace {
    /// Lifecycle and pull-request state.
    pub status: WorkspaceStatus,
    /// The branch name.
    pub branch: String,
    /// Added lines, zero when none.
    pub added: u32,
    /// Deleted lines, zero when none.
    pub deleted: u32,
    /// The path of another checkout that holds the branch, if any.
    pub held_by_path: Option<String>,
}
