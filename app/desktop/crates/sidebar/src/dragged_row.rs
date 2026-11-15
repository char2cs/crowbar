//! The payload gpui carries while a tree row is dragged.

use gpui::SharedString;

use crate::row_id::RowId;

#[derive(Debug, Clone)]
pub(crate) struct DraggedRow {
    pub(crate) row: RowId,
    pub(crate) label: SharedString,
}
