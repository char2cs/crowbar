//! What a confirmed deletion removes.

use crate::project_id::ProjectId;
use crate::row_id::RowId;

/// What a confirmed deletion removes.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RemovalTarget {
    /// A tree row.
    Row(RowId),
    /// A whole space.
    Project(ProjectId),
}
