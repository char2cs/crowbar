use std::collections::HashMap;
use std::sync::{Mutex, MutexGuard};

use crate::error::Error;
use crate::token_store::TokenStore;

/// A [`TokenStore`] that keeps entries in memory only.
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
    fn get(&self, host_id: &str) -> Result<Option<String>, Error> {
        Ok(self.entries().get(host_id).cloned())
    }

    fn set(&self, host_id: &str, entry: &str) -> Result<(), Error> {
        self.entries().insert(host_id.to_owned(), entry.to_owned());
        Ok(())
    }

    fn delete(&self, host_id: &str) -> Result<(), Error> {
        self.entries().remove(host_id);
        Ok(())
    }
}

#[cfg(test)]
#[path = "memory_token_store_tests.rs"]
mod tests;
