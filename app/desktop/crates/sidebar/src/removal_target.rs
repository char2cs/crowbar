//! What a confirmed deletion removes.

use crate::project_id::ProjectId;
use crate::row_id::RowId;

/// The thing a confirmed deletion removes.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RemovalTarget {
    /// A row of the chats tree.
    Row(RowId),
    /// A whole space.
    Project(ProjectId),
}
