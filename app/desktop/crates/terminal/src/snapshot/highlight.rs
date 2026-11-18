//! A horizontal stretch of one visible row marked for the view to tint.

/// Columns `start_column..end_column` of `row`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Highlight {
    /// Visible row.
    pub row: usize,
    /// First marked column.
    pub start_column: usize,
    /// One past the last marked column.
    pub end_column: usize,
}
