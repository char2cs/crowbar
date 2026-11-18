//! A row that survived folding and removal, ready to draw.

use crate::shell::Expansion;
use crate::tree::deletion::Hold;

/// A tree row to draw, with how it is folded and any removal pending on it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct VisibleRow {
    /// The row's position in the full list of rows.
    pub index: usize,
    /// Whether the row shows its children.
    pub expansion: Expansion,
    /// The removal counting down on the row, if any.
    pub removal: Option<Hold>,
}
