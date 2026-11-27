//! The inline input a tree row is showing, if any.

use crate::shell::NewEntry;
use crate::shell::RowId;

use crate::shell::ProjectId;
/// Which name the sidebar's inline input is collecting; at most one is open at a time.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum Editing {
    /// No input is open.
    #[default]
    Nothing,
    /// The row's new name.
    Renaming(RowId),
    /// The space's new name, typed over its name in the switcher.
    RenamingProject(ProjectId),
    /// The name of the branch the chat row turns into.
    Branching(RowId),
    /// The name of a fork of the row.
    Drafting(RowId),
    /// The new name of the file at this workspace-relative path.
    RenamingFile(String),
    /// The name of a new file or folder.
    Creating {
        /// The directory it is created in.
        dir: String,
        /// Whether it is a file or a folder.
        entry: NewEntry,
    },
}

impl Editing {
    pub(crate) fn row(&self) -> Option<&RowId> {
        match self {
            Self::Nothing
            | Self::RenamingProject(_)
            | Self::RenamingFile(_)
            | Self::Creating { .. } => None,
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
        let file_edits = [
            Editing::RenamingFile("a.rs".into()),
            Editing::Creating {
                dir: String::new(),
                entry: NewEntry::File,
            },
        ];
        for editing in file_edits {
            assert_eq!(editing.row(), None, "{editing:?} belongs to no tree row");
        }
        for editing in [
            Editing::Renaming(row.clone()),
            Editing::Branching(row.clone()),
            Editing::Drafting(row.clone()),
        ] {
            assert_eq!(editing.row(), Some(&row), "{editing:?}");
        }
    }
}
