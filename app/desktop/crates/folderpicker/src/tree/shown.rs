//! What the tree is showing: the listings of the data and the folders the user opened.

use std::collections::BTreeSet;

use crate::folder_source::FolderSource;

pub(crate) struct Shown<'a> {
    pub(crate) source: &'a dyn FolderSource,
    pub(crate) expanded: &'a BTreeSet<String>,
}
