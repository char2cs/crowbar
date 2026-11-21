//! One line of the Changes tree.

use crate::shell::Expansion;
use crate::shell::git::changed_file::ChangedFile;

/// One line of the Changes tree, after folder chains are joined and folded folders hide their
/// contents.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ChangedRow {
    /// A folder; a chain of levels that hold one folder and no file shows as one `a/b` row.
    Folder {
        /// The deepest folder's path: the chain's identity.
        path: String,
        /// The name, `a/b` for a joined chain.
        label: String,
        /// The nesting level.
        depth: u8,
        /// Whether its contents show.
        expansion: Expansion,
        /// Every changed file under it, path and signature, for ticking them all at once.
        files: Vec<(String, String)>,
        /// How many of `files` are viewed.
        viewed: usize,
    },
    /// A changed file.
    File {
        /// The file.
        file: ChangedFile,
        /// Its base name.
        name: String,
        /// The nesting level.
        depth: u8,
        /// Unresolved review threads anchored on it.
        threads: u32,
        /// Marked viewed, and unchanged since.
        viewed: bool,
    },
}

impl ChangedRow {
    /// The nesting level of the folder or file.
    #[must_use]
    pub fn depth(&self) -> u8 {
        match self {
            Self::Folder { depth, .. } | Self::File { depth, .. } => *depth,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_folder_and_a_file_report_their_level() {
        let folder = ChangedRow::Folder {
            path: "src".into(),
            label: "src".into(),
            depth: 1,
            expansion: Expansion::Expanded,
            files: Vec::new(),
            viewed: 0,
        };
        let file = ChangedRow::File {
            file: ChangedFile::default(),
            name: "a.ts".into(),
            depth: 2,
            threads: 0,
            viewed: false,
        };
        assert_eq!((folder.depth(), file.depth()), (1, 2));
    }
}
