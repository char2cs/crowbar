//! A cell position in the visible viewport.

/// `row` counts from the top visible line, `column` from the left; both start at zero.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Hash)]
pub struct ViewPoint {
    /// Visible line, 0 at the top.
    pub row: usize,
    /// Column, 0 at the left.
    pub column: usize,
}
