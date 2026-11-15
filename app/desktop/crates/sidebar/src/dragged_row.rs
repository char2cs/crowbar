//! The payload gpui carries while a tree row is dragged.

use crate::row_ghost::GhostBuild;
use crate::row_id::RowId;

#[derive(Clone)]
pub(crate) struct DraggedRow {
    pub(crate) row: RowId,
    pub(crate) ghost: GhostBuild,
}
