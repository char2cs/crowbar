use std::collections::HashMap;
use std::sync::{Mutex, MutexGuard, PoisonError};

use crate::error::Error;
use crate::keychain::Keychain;
use crate::read_error::ReadError;
use crate::token_store::TokenStore;

type Remembered = Result<Option<String>, String>;

/// Reads each host's entry from the keychain at most once per process, because every read can
/// raise its own OS access prompt and callers read on every request and socket open. Found,
/// missing and user-denied outcomes are remembered; pairing again clears a denial. Any other
/// failure (a locked keychain) is retried. The lock is held across a read so concurrent first
/// reads prompt once.
#[derive(Debug)]
pub struct CachedTokenStore<K: Keychain> {
    keychain: K,
    cache: Mutex<HashMap<String, Remembered>>,
}

impl<K: Keychain> CachedTokenStore<K> {
    /// Wraps `keychain` with an empty cache.
    pub fn new(keychain: K) -> Self {
        Self {
            keychain,
            cache: Mutex::default(),
        }
    }

    fn cache(&self) -> MutexGuard<'_, HashMap<String, Remembered>> {
        self.cache.lock().unwrap_or_else(PoisonError::into_inner)
    }
}

impl<K: Keychain> TokenStore for CachedTokenStore<K> {
    fn get(&self, host_id: &str) -> Result<Option<String>, Error> {
        let mut cache = self.cache();
        if let Some(cached) = cache.get(host_id) {
            return cached.clone().map_err(Error::Store);
        }
        let outcome = match self.keychain.read(host_id) {
            Ok(entry) => Ok(entry),
            Err(ReadError {
                message,
                denied: false,
            }) => return Err(Error::Store(message)),
            Err(ReadError { message, .. }) => Err(message),
        };
        cache.insert(host_id.to_owned(), outcome.clone());
        outcome.map_err(Error::Store)
    }

    fn set(&self, host_id: &str, entry: &str) -> Result<(), Error> {
        let mut cache = self.cache();
        self.keychain.write(host_id, entry)?;
        cache.insert(host_id.to_owned(), Ok(Some(entry.to_owned())));
        Ok(())
    }

    fn delete(&self, host_id: &str) -> Result<(), Error> {
        let mut cache = self.cache();
        self.keychain.remove(host_id)?;
        cache.insert(host_id.to_owned(), Ok(None));
        Ok(())
    }
}

#[cfg(test)]
#[path = "cached_token_store_tests.rs"]
mod tests;
