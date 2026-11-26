//! What the pointer is over while a Files row is dragged.

use crate::shell::FileKind;

/// The part of the Files panel under a dragged row.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DropSpot {
    /// The panel away from any row: the workspace root.
    Root,
    /// A row of the tree.
    Row {
        /// The row's path.
        path: String,
        /// Whether it is a file or a folder, and its fold.
        kind: FileKind,
    },
}

impl DropSpot {
    /// The folder a drop here lands in: the row's own folder, the folder holding a file row, or
    /// the root (an empty path).
    pub fn directory(&self) -> &str {
        match self {
            Self::Root => "",
            Self::Row {
                path,
                kind: FileKind::Directory(_),
            } => path,
            Self::Row { path, .. } => crate::shell::PathText::new(path).parent(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::Expansion;

    fn row(
        path: &str,
        kind: FileKind,
    ) -> DropSpot {
        DropSpot::Row {
            path: path.into(),
            kind,
        }
    }

    #[test]
    fn a_folder_takes_the_drop_a_file_hands_it_to_its_folder_and_the_root_is_empty() {
        let cases = [
            (row("src", FileKind::Directory(Expansion::Collapsed)), "src"),
            (row("src/a.rs", FileKind::File), "src"),
            (row("README.md", FileKind::File), ""),
            (DropSpot::Root, ""),
        ];
        for (spot, want) in cases {
            assert_eq!(spot.directory(), want, "{spot:?}");
        }
    }
}
