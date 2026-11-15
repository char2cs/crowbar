//! The press that may turn into a drag.

use gpui::{Pixels, Point};

use crate::row_id::RowId;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct DragPress {
    pub(crate) row: RowId,
    pub(crate) at: Point<Pixels>,
}
