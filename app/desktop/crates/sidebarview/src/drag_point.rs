//! Where the pointer is over a target row while another row is dragged.

use gpui::{Bounds, Pixels, Point};

use sidebar::RowId;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct DragPoint {
    pub(crate) subject: RowId,
    pub(crate) target: RowId,
    pub(crate) bounds: Bounds<Pixels>,
    pub(crate) at: Point<Pixels>,
}
