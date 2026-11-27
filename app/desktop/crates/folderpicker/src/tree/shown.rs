use std::collections::BTreeSet;

use crate::folder_source::FolderSource;

pub(crate) struct Shown<'a> {
    pub(crate) source: &'a dyn FolderSource,
    pub(crate) expanded: &'a BTreeSet<String>,
}
