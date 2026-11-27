use std::time::Instant;

use layout::{DragSubject, PaneSlot};
use protocol::PaneId;

#[derive(Clone, Copy)]
pub(crate) struct DropHit<'a> {
    pub(crate) subject: &'a DragSubject,
    pub(crate) point: (f32, f32),
    pub(crate) slots: &'a [PaneSlot],
    pub(crate) joinable: &'a [PaneId],
    pub(crate) now: Instant,
}
