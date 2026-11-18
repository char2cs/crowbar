//! A filled rectangle behind cells.

use crate::snapshot::palette::Rgb;

/// `columns` cells from `column` filled with a non-default background.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct BackgroundSpan {
    /// First filled column.
    pub column: usize,
    /// Number of filled columns.
    pub columns: usize,
    /// Fill colour.
    pub color: Rgb,
}
