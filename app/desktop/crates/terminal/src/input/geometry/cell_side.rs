//! Which half of a cell a pointer is over.

/// The half of a cell under the pointer; selections include a cell only from its covered half.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CellSide {
    /// Left half.
    Left,
    /// Right half.
    Right,
}
