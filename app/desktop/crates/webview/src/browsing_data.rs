use crate::error::Result;

/// The stored cookies, cache and site data of browser views.
pub trait BrowsingData {
    /// Deletes cookies, cache and site storage of the browser views' own store. Core and
    /// extension views keep theirs.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or the engine refuses.
    fn clear_browsing_data(&self) -> Result<()>;
}
