//! What a card is: seen, hit by the pointer, or both.

use crate::floating::layers::float_host::float_region::FloatRegion;

pub(crate) type Cards = Vec<(Part, FloatRegion)>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Part {
    /// Painted, and the pointer acts on it.
    Solid,
    /// Painted; the pointer passes through to the window below.
    Seen,
    /// Unpainted; it catches the pointer, as a click-away layer does.
    Catch,
}
