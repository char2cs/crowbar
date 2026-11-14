use crate::error::Error;

/// Where bearer tokens live. Implemented by the OS keychain in the app and by an in-memory map
/// in tests. Reads and writes may block on an OS prompt, so async callers use the blocking
/// pool. Entries are [`Credential::encode`](crate::Credential::encode)d strings.
pub trait TokenStore: Send + Sync {
    /// The entry for `host_id`, `None` when there is none.
    ///
    /// # Errors
    /// [`Error::Store`] when the store cannot be read.
    fn get(&self, host_id: &str) -> Result<Option<String>, Error>;

    /// Replaces the entry for `host_id`.
    ///
    /// # Errors
    /// [`Error::Store`] when the store cannot be written.
    fn set(&self, host_id: &str, entry: &str) -> Result<(), Error>;

    /// Removes the entry for `host_id`; removing none is not an error.
    ///
    /// # Errors
    /// [`Error::Store`] when the store cannot be written.
    fn delete(&self, host_id: &str) -> Result<(), Error>;
}

#[cfg(test)]
#[path = "token_store_tests.rs"]
mod tests;
