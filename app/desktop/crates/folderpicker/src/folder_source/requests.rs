//! What the picker asks of the data behind it.

pub(crate) trait Requests {
    fn request(
        &self,
        path: &str,
    );

    fn retry(
        &self,
        path: &str,
    );
}
