//! One visible row of the Files panel.

use crate::file_kind::FileKind;
use crate::git_status::GitStatus;

/// A file or directory of the focused workspace, in tree order.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FileRow {
    /// Path relative to the workspace root.
    pub path: String,
    /// The base name shown.
    pub name: String,
    /// Nesting depth, zero at the root.
    pub depth: u8,
    /// File or directory.
    pub kind: FileKind,
    /// Git decoration.
    pub status: GitStatus,
}
