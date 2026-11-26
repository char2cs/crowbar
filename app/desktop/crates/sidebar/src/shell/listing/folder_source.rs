//! Whether the Files panel has a folder, and whether its root is listed yet.

use crate::shell::listing::folder_phase::FolderPhase;

/// Whether the Files panel has a folder, and whether its root is listed yet.
pub trait FolderSource {
    /// How far the followed workspace's folder has come.
    fn folder_phase(&self) -> FolderPhase;
}
