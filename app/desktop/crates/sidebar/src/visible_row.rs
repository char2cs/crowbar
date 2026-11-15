//! A row that survived folding and removal, ready to draw.

use crate::expansion::Expansion;
use crate::hold::Hold;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct VisibleRow {
    pub(crate) index: usize,
    pub(crate) expansion: Expansion,
    pub(crate) removal: Option<Hold>,
}
