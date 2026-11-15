//! A directory's listing as far as the host has answered.

use crate::browse_entry::BrowseEntry;

/// Asked for, refused, or read.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum FolderListing {
    /// The request is on its way.
    Loading,
    /// The host refused or could not be reached; the reason is fit to show.
    Failed(String),
    /// The children of the directory, folders first.
    Loaded(Vec<BrowseEntry>),
}
