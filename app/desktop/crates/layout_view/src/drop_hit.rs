use gpui::{Bounds, Pixels};
use model::{DragSubject, PaneSlot};

#[derive(Debug, Clone, Copy)]
pub(crate) struct DropHit<'a> {
    pub(crate) subject: &'a DragSubject,
    pub(crate) point: (f32, f32),
    pub(crate) slots: &'a [PaneSlot],
    pub(crate) window_bounds: Bounds<Pixels>,
}
