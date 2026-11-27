use std::collections::HashMap;
use std::sync::{Mutex, MutexGuard, PoisonError};

use crate::error::Error;
use crate::transport::remote::secrets::keychain::Keychain;
use crate::transport::remote::secrets::read_error::ReadError;
use crate::transport::remote::secrets::token_store::TokenStore;

type Remembered = Result<Option<String>, ReadError>;

#[derive(Debug)]
pub struct CachedTokenStore<K: Keychain> {
    keychain: K,
    cache: Mutex<HashMap<String, Remembered>>,
}

impl<K: Keychain> CachedTokenStore<K> {
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

fn stored(failure: ReadError) -> Error {
    Error::Store(failure.message)
}

impl<K: Keychain> TokenStore for CachedTokenStore<K> {
    fn load_entry(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, Error> {
        let mut cache = self.cache();
        if let Some(cached) = cache.get(host_id) {
            return cached.clone().map_err(stored);
        }
        let outcome = match self.keychain.read(host_id) {
            Err(refusal) if !refusal.denied => return Err(stored(refusal)),
            read => read,
        };
        cache.insert(host_id.to_owned(), outcome.clone());
        outcome.map_err(stored)
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
        refuse_writes: Mutex<bool>,
    }

    impl Counting {
        fn fail_with(
            &self,
            failure: Option<Failure>,
        ) {
            *self.failure.lock().unwrap() = failure;
        }

        fn refuse_writes(&self) {
            *self.refuse_writes.lock().unwrap() = true;
        }

        fn ensure_writable(&self) -> Result<(), Error> {
            let refused = *self.refuse_writes.lock().unwrap();
            (!refused)
                .then_some(())
                .ok_or_else(|| Error::Store("no secret service".into()))
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
            self.ensure_writable()?;
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
    fn a_keychain_that_refuses_the_write_is_an_error_and_nothing_is_kept_in_memory() {
        let (keychain, store) = store();
        keychain.refuse_writes();
        let refused = store.set("h", "e").unwrap_err();
        assert!(refused.to_string().contains("no secret service"));
        assert_eq!(store.load_entry("h").unwrap(), None);
        assert_eq!(keychain.reads.load(Ordering::SeqCst), 1);
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
