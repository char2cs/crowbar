use crate::error::Error;

pub trait TokenStore: Send + Sync {
    fn load_entry(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, Error>;

    fn set(
        &self,
        host_id: &str,
        entry: &str,
    ) -> Result<(), Error>;

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
