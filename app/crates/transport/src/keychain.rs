use crate::error::Error;
use crate::read_error::ReadError;

/// Raw keychain access, split from the cache in front of it so the caching can be tested without
/// touching an OS keychain.
pub trait Keychain: Send + Sync {
    /// Reads one entry.
    ///
    /// # Errors
    /// A [`ReadError`] saying whether the user refused.
    fn read(&self, host_id: &str) -> Result<Option<String>, ReadError>;

    /// Writes one entry.
    ///
    /// # Errors
    /// [`Error::Store`] when the keychain refuses the write.
    fn write(&self, host_id: &str, entry: &str) -> Result<(), Error>;

    /// Removes one entry; a missing one is not an error.
    ///
    /// # Errors
    /// [`Error::Store`] when the keychain refuses the removal.
    fn remove(&self, host_id: &str) -> Result<(), Error>;
}

#[cfg(test)]
#[path = "keychain_tests.rs"]
mod tests;
