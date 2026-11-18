//! One visible row of the tree.

use shell_data::BrowseEntry;
use ui::Pose;

use crate::folder_note::FolderNote;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum FolderRow {
    Entry {
        entry: BrowseEntry,
        level: usize,
        open: Pose,
    },
    Note {
        level: usize,
        note: FolderNote,
    },
}
