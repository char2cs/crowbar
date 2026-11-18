use std::time::Instant;

use gpui::{Bounds, Pixels};
use layout::{DragSubject, PaneSlot};
use protocol::PaneId;

#[derive(Clone, Copy)]
pub(crate) struct DropHit<'a> {
    pub(crate) subject: &'a DragSubject,
    pub(crate) point: (f32, f32),
    pub(crate) slots: &'a [PaneSlot],
    pub(crate) joinable: &'a [PaneId],
    pub(crate) window_bounds: Bounds<Pixels>,
    pub(crate) now: Instant,
}
