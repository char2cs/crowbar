//! The inline input a tree row is showing, if any.

use crate::row_id::RowId;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub(crate) enum Editing {
    #[default]
    Nothing,
    Renaming(RowId),
    Branching(RowId),
    Drafting(RowId),
}

impl Editing {
    pub(crate) fn row(&self) -> Option<&RowId> {
        match self {
            Self::Nothing => None,
            Self::Renaming(row) | Self::Branching(row) | Self::Drafting(row) => Some(row),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_owner_row_is_exposed() {
        let row = RowId::from("r");
        assert_eq!(Editing::Nothing.row(), None, "idle");
        for editing in [
            Editing::Renaming(row.clone()),
            Editing::Branching(row.clone()),
            Editing::Drafting(row.clone()),
        ] {
            assert_eq!(editing.row(), Some(&row), "{editing:?}");
        }
    }
}
