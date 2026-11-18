//! What a row of the Files or Changes panel shows.

use sidebar::{FileKind, GitStatus};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct FileTreeEntry {
    pub(crate) path: String,
    pub(crate) name: String,
    pub(crate) depth: u8,
    pub(crate) kind: FileKind,
    pub(crate) status: GitStatus,
    pub(crate) counts: Option<(u32, u32)>,
}
