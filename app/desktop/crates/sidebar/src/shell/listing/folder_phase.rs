//! How far the Files panel's folder has come: none, asked for, or listed.

/// How far the Files panel's folder has come.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FolderPhase {
    /// No workspace is followed, so there is no folder to show.
    NoFolder,
    /// The folder's root was asked for and has not been listed yet.
    Loading,
    /// The root is listed; an empty listing is an empty folder.
    Listed,
}
