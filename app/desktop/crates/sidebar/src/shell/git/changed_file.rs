//! One entry of the Changes panel.

use crate::shell::listing::GitStatus;

/// A file the branch changed.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ChangedFile {
    /// Path relative to the repository.
    pub path: String,
    /// Git status.
    pub status: GitStatus,
    /// Added lines.
    pub added: u32,
    /// Deleted lines.
    pub deleted: u32,
}
