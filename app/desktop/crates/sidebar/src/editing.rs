//! The inline input a tree row is showing, if any.

use crate::row_id::RowId;

/// Renaming and branching are mutually exclusive: starting one stops the other.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum Editing {
    /// No inline input.
    #[default]
    Nothing,
    /// The label of the row is an input.
    Renaming(RowId),
    /// The label of the chat row is the "make a branch" input.
    Branching(RowId),
    /// A naming input sits where the not-yet-created fork of the row will appear.
    Drafting(RowId),
}

impl Editing {
    /// The row that owns the input.
    pub fn row(&self) -> Option<&RowId> {
        match self {
            Self::Nothing => None,
            Self::Renaming(row) | Self::Branching(row) | Self::Drafting(row) => Some(row),
        }
    }
}

#[cfg(test)]
#[path = "editing_tests.rs"]
mod tests;
