use gpui::{Pixels, Point};

use sidebar::RowId;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct DragPress {
    pub(crate) row: RowId,
    pub(crate) at: Point<Pixels>,
}
