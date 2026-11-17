//! What the picker asks of the data behind it.

pub(crate) trait FolderRequests {
    fn request(
        &self,
        path: &str,
    );

    fn retry(
        &self,
        path: &str,
    );
}
