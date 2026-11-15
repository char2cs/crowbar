//! What a confirmed deletion removes.

use crate::project_id::ProjectId;
use crate::row_id::RowId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum RemovalTarget {
    Row(RowId),
    Project(ProjectId),
}
