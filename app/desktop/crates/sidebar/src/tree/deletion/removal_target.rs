//! What a confirmed deletion removes.

use crate::shell::ProjectId;
use crate::shell::RowId;

/// The thing a confirmed deletion removes.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RemovalTarget {
    /// A row of the chats tree.
    Row(RowId),
    /// A whole space.
    Project(ProjectId),
}
