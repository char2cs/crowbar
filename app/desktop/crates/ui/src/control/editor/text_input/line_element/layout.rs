//! Alignment and the line a single-line field stores after painting.

use gpui::{Bounds, Pixels};

use crate::display::TrackedLine;

pub(crate) trait Layout {
    fn centered(&self) -> bool;
    /// Letter spacing in ems.
    fn tracking(&self) -> f32;
    fn record_layout(
        &mut self,
        line: TrackedLine,
        placed: (Bounds<Pixels>, Pixels),
    );
}
