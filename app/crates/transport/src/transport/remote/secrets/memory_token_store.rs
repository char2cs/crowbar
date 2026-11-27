use std::collections::HashMap;
use std::sync::{Mutex, MutexGuard};

use crate::error::Error;
use crate::transport::remote::secrets::token_store::TokenStore;

#[derive(Debug, Default)]
pub struct MemoryTokenStore(Mutex<HashMap<String, String>>);

impl MemoryTokenStore {
    fn entries(&self) -> MutexGuard<'_, HashMap<String, String>> {
        self.0
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
    }
}

impl TokenStore for MemoryTokenStore {
    fn load_entry(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, Error> {
        Ok(self.entries().get(host_id).cloned())
    }

    fn set(
        &self,
        host_id: &str,
        entry: &str,
    ) -> Result<(), Error> {
        self.entries().insert(host_id.to_owned(), entry.to_owned());
        Ok(())
    }

    fn delete(
        &self,
        host_id: &str,
    ) -> Result<(), Error> {
        self.entries().remove(host_id);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_memory_store_round_trips() {
        let store = MemoryTokenStore::default();
        store.set("h", "e").unwrap();
        assert_eq!(store.load_entry("h").unwrap(), Some("e".into()));
        store.delete("h").unwrap();
        assert_eq!(store.load_entry("h").unwrap(), None);
    }
}
