use crate::error::Error;

/// Where bearer tokens live. Implemented by the OS keychain in the app and by an in-memory map
/// in tests. Reads and writes may block on an OS prompt, so async callers use the blocking
/// pool. Entries are strings produced by `Credential::encode`.
pub trait TokenStore: Send + Sync {
    /// The entry for `host_id`, `None` when there is none.
    ///
    /// # Errors
    /// [`Error::Store`] when the store cannot be read.
    fn load_entry(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, Error>;

    /// Replaces the entry for `host_id`.
    ///
    /// # Errors
    /// [`Error::Store`] when the store cannot be written.
    fn set(
        &self,
        host_id: &str,
        entry: &str,
    ) -> Result<(), Error>;

    /// Removes the entry for `host_id`; removing none is not an error.
    ///
    /// # Errors
    /// [`Error::Store`] when the store cannot be written.
    fn delete(
        &self,
        host_id: &str,
    ) -> Result<(), Error>;
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::transport::remote::secrets::memory_token_store::MemoryTokenStore;

    #[test]
    fn a_store_is_usable_as_a_shared_trait_object_and_forgets_on_delete() {
        let store: Arc<dyn TokenStore> = Arc::new(MemoryTokenStore::default());
        assert_eq!(store.load_entry("h").unwrap(), None);
        store.set("h", "entry").unwrap();
        assert_eq!(store.load_entry("h").unwrap().as_deref(), Some("entry"));
        store.delete("h").unwrap();
        assert_eq!(store.load_entry("h").unwrap(), None);
    }
}
