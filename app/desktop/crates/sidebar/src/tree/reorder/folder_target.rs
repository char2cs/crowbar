//! Whether the row under the drag pointer is a folder.

/// Folders have a narrower before/after band than other rows.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FolderTarget {
    /// The target is a folder.
    Yes,
    /// Any other row.
    No,
}
