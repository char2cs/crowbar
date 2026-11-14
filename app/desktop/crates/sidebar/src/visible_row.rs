//! A row that survived folding and removal, ready to draw.

use crate::expansion::Expansion;
use crate::hold::Hold;

/// Index into the data source's rows plus what the shell adds to it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct VisibleRow {
    /// Position in the rows the data source returned.
    pub index: usize,
    /// Fold state of a parent; `Expanded` for leaves.
    pub expansion: Expansion,
    /// The countdown the row is drawn as, when it is being removed.
    pub removal: Option<Hold>,
}
