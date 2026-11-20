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
    },
}
