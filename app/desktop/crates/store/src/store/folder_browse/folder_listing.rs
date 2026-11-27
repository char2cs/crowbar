use crate::store::folder_browse::browse_entry::BrowseEntry;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum FolderListing {
    Loading,
    Failed(String),
    Loaded(Vec<BrowseEntry>),
}
