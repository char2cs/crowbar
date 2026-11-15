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
    fn load_entry(
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
    use std::sync::atomic::{AtomicUsize, Ordering};

    use super::*;

    #[derive(Clone, Copy)]
    enum Failure {
        Denied,
        Locked,
    }

    fn read_error(failure: Failure) -> ReadError {
        match failure {
            Failure::Denied => ReadError {
                message: "user denied".into(),
                denied: true,
            },
            Failure::Locked => ReadError {
                message: "keychain is locked".into(),
                denied: false,
            },
        }
    }

    #[derive(Default)]
    struct Counting {
        entries: Mutex<HashMap<String, String>>,
        reads: AtomicUsize,
        failure: Mutex<Option<Failure>>,
    }

    impl Counting {
        fn fail_with(
            &self,
            failure: Option<Failure>,
        ) {
            *self.failure.lock().unwrap() = failure;
        }
    }

    impl Keychain for Arc<Counting> {
        fn read(
            &self,
            host_id: &str,
        ) -> Result<Option<String>, ReadError> {
            self.reads.fetch_add(1, Ordering::SeqCst);
            let failure = *self.failure.lock().unwrap();
            failure.map_or_else(
                || Ok(self.entries.lock().unwrap().get(host_id).cloned()),
                |failure| Err(read_error(failure)),
            )
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
            assert_eq!(store.load_entry("h").unwrap(), Some("e".into()));
            assert_eq!(store.load_entry("missing").unwrap(), None);
        }
        assert_eq!(keychain.reads.load(Ordering::SeqCst), 2);
    }

    /// (failure, keychain reads after two lookups): a denial is remembered, a locked keychain is retried.
    const CASES: [(Failure, usize); 2] = [(Failure::Denied, 1), (Failure::Locked, 2)];

    #[test]
    fn only_a_denial_is_remembered_other_failures_are_retried() {
        for (failure, reads) in CASES {
            let (keychain, store) = store();
            keychain.fail_with(Some(failure));
            assert!(store.load_entry("h").is_err());
            assert!(store.load_entry("h").is_err());
            assert_eq!(keychain.reads.load(Ordering::SeqCst), reads);
        }
    }

    #[test]
    fn pairing_again_clears_a_remembered_denial() {
        let (keychain, store) = store();
        keychain.fail_with(Some(Failure::Denied));
        assert!(store.load_entry("h").is_err());
        store.set("h", "fresh").unwrap();
        assert_eq!(store.load_entry("h").unwrap(), Some("fresh".into()));
    }

    #[test]
    fn a_retried_failure_recovers_once_the_keychain_unlocks() {
        let (keychain, store) = store();
        keychain.fail_with(Some(Failure::Locked));
        assert!(store.load_entry("h").is_err());
        keychain.fail_with(None);
        assert_eq!(store.load_entry("h").unwrap(), None);
    }

    #[test]
    fn delete_forgets_without_reading_again() {
        let (keychain, store) = store();
        store.set("h", "e").unwrap();
        store.delete("h").unwrap();
        assert_eq!(store.load_entry("h").unwrap(), None);
        assert_eq!(keychain.reads.load(Ordering::SeqCst), 0);
    }
}
