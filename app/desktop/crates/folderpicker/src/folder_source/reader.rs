use store::FolderListing;

pub(crate) trait Reader {
    fn host_name(&self) -> String;

    fn can_ask(&self) -> bool;

    fn listing(
        &self,
        path: &str,
    ) -> Option<FolderListing>;
}
