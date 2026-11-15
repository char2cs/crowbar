//! What the picker needs of the data behind it, so the data can be replaced in tests.

use shell_data::FolderListing;

pub(crate) trait FolderSource {
    fn host_name(&self) -> String;

    fn can_ask(&self) -> bool;

    fn listing(
        &self,
        path: &str,
    ) -> Option<FolderListing>;

    fn request(
        &self,
        path: &str,
    );

    fn retry(
        &self,
        path: &str,
    );
}
