//! One child of a directory on a host's own disk.

use daemonapi::EntryKind;

/// An entry of a listed directory.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BrowseEntry {
    /// The file name.
    pub name: String,
    /// The absolute path, under the listed directory.
    pub path: String,
    /// Whether it can be opened and chosen.
    pub kind: EntryKind,
}
