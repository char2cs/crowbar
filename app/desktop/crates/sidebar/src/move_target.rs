//! A container a row may be moved into.

use crate::row_id::RowId;
use crate::row_kind::RowKind;

/// A legal "Move to…" destination: a folder or branch of the same space.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MoveTarget {
    /// The container.
    pub id: RowId,
    /// The branch name or label.
    pub label: String,
    /// Folder or branch; picks the icon.
    pub kind: RowKind,
}
