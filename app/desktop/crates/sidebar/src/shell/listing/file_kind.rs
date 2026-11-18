//! Files and directories of the file tree.

use crate::shell::chats::Expansion;

/// A file, or a directory with its fold state.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FileKind {
    /// A regular file.
    File,
    /// A directory.
    Directory(Expansion),
}
