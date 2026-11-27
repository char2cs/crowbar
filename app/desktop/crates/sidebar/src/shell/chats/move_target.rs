use crate::shell::chats::row_id::RowId;
use crate::shell::chats::row_kind::RowKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MoveTarget {
    pub id: RowId,
    pub label: String,
    pub kind: RowKind,
}
