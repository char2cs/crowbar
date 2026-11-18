//! Whether a listed entry of a host's disk is a folder or a file.

/// A folder can be opened and chosen; a file is only shown.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EntryKind {
    /// A directory, or a symlink to one.
    Folder,
    /// Anything else.
    File,
}
