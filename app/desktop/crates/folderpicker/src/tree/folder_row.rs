use store::BrowseEntry;
use ui::Pose;

use crate::tree::folder_note::FolderNote;

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
