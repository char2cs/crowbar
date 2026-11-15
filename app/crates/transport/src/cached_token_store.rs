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
    fn get(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, Error> {
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

    fn set(
        &self,
        host_id: &str,
        entry: &str,
    ) -> Result<(), Error> {
        let mut cache = self.cache();
        self.keychain.write(host_id, entry)?;
        cache.insert(host_id.to_owned(), Ok(Some(entry.to_owned())));
        Ok(())
    }

    fn delete(
        &self,
        host_id: &str,
    ) -> Result<(), Error> {
        let mut cache = self.cache();
        self.keychain.remove(host_id)?;
        cache.insert(host_id.to_owned(), Ok(None));
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};

    use super::*;

    #[derive(Default)]
    struct Counting {
        entries: Mutex<HashMap<String, String>>,
        reads: AtomicUsize,
        deny: AtomicBool,
        transient: AtomicBool,
    }

    impl Keychain for Arc<Counting> {
        fn read(
            &self,
            host_id: &str,
        ) -> Result<Option<String>, ReadError> {
            self.reads.fetch_add(1, Ordering::SeqCst);
            if self.deny.load(Ordering::SeqCst) {
                return Err(ReadError {
                    message: "user denied".into(),
                    denied: true,
                });
            }
            if self.transient.load(Ordering::SeqCst) {
                return Err(ReadError {
                    message: "keychain is locked".into(),
                    denied: false,
                });
            }
            Ok(self.entries.lock().unwrap().get(host_id).cloned())
        }

        fn write(
            &self,
            host_id: &str,
            entry: &str,
        ) -> Result<(), Error> {
            self.entries
                .lock()
                .unwrap()
                .insert(host_id.into(), entry.into());
            Ok(())
        }

        fn remove(
            &self,
            host_id: &str,
        ) -> Result<(), Error> {
            self.entries.lock().unwrap().remove(host_id);
            Ok(())
        }
    }

    fn store() -> (Arc<Counting>, CachedTokenStore<Arc<Counting>>) {
        let keychain = Arc::new(Counting::default());
        (Arc::clone(&keychain), CachedTokenStore::new(keychain))
    }

    #[test]
    fn each_entry_is_read_from_the_keychain_once() {
        let (keychain, store) = store();
        keychain
            .entries
            .lock()
            .unwrap()
            .insert("h".into(), "e".into());
        for _ in 0..3 {
            assert_eq!(store.get("h").unwrap(), Some("e".into()));
            assert_eq!(store.get("missing").unwrap(), None);
        }
        assert_eq!(keychain.reads.load(Ordering::SeqCst), 2);
    }

    #[test]
    fn a_denial_is_remembered_until_the_host_is_paired_again() {
        let (keychain, store) = store();
        keychain.deny.store(true, Ordering::SeqCst);
        assert!(store.get("h").is_err());
        assert!(store.get("h").is_err());
        assert_eq!(keychain.reads.load(Ordering::SeqCst), 1);
        store.set("h", "fresh").unwrap();
        assert_eq!(store.get("h").unwrap(), Some("fresh".into()));
    }

    #[test]
    fn a_transient_failure_is_retried() {
        let (keychain, store) = store();
        keychain.transient.store(true, Ordering::SeqCst);
        assert!(store.get("h").is_err());
        keychain.transient.store(false, Ordering::SeqCst);
        assert_eq!(store.get("h").unwrap(), None);
        assert_eq!(keychain.reads.load(Ordering::SeqCst), 2);
    }

    #[test]
    fn delete_forgets_without_reading_again() {
        let (keychain, store) = store();
        store.set("h", "e").unwrap();
        store.delete("h").unwrap();
        assert_eq!(store.get("h").unwrap(), None);
        assert_eq!(keychain.reads.load(Ordering::SeqCst), 0);
    }
}
