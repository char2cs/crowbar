//! What the picker reads of the data behind it.

use shell_data::FolderListing;

pub(crate) trait FolderReader {
    fn host_name(&self) -> String;

    fn can_ask(&self) -> bool;

    fn listing(
        &self,
        path: &str,
    ) -> Option<FolderListing>;
}
