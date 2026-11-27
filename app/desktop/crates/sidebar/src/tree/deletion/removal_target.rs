use crate::shell::ProjectId;
use crate::shell::RowId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RemovalTarget {
    Row(RowId),
    Project(ProjectId),
}
