use std::collections::HashMap;
use std::sync::{Mutex, MutexGuard};

use super::keychain::Keychain;
use super::read_error::ReadError;
use super::token_store::TokenStore;

type Cache = HashMap<String, Result<Option<String>, String>>;

pub(super) struct KeychainTokenStore<K: Keychain> {
    keychain: K,
    cache: Mutex<Cache>,
}

impl<K: Keychain> KeychainTokenStore<K> {
    pub(super) fn new(keychain: K) -> Self {
        Self {
            keychain,
            cache: Mutex::default(),
        }
    }

    fn cache(&self) -> MutexGuard<'_, Cache> {
        self.cache
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
    }
}

impl<K: Keychain> TokenStore for KeychainTokenStore<K> {
    fn get(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, String> {
        let mut cache = self.cache();
        if let Some(cached) = cache.get(host_id) {
            return cached.clone();
        }
        let stored = match self.keychain.read(host_id) {
            Ok(entry) => Ok(entry),
            Err(ReadError {
                message,
                denied: false,
            }) => return Err(message),
            Err(ReadError { message, .. }) => Err(message),
        };
        cache.insert(host_id.to_owned(), stored.clone());
        stored
    }

    fn set(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), String> {
        let mut cache = self.cache();
        self.keychain.write(host_id, token)?;
        cache.insert(host_id.to_owned(), Ok(Some(token.to_owned())));
        Ok(())
    }

    fn delete(
        &self,
        host_id: &str,
    ) -> Result<(), String> {
        let mut cache = self.cache();
        self.keychain.remove(host_id)?;
        cache.insert(host_id.to_owned(), Ok(None));
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Arc;

    #[derive(Clone, Copy)]
    enum Mode {
        Normal,
        Denied,
        Locked,
    }

    struct CountingKeychain {
        entries: Mutex<HashMap<String, String>>,
        reads: AtomicUsize,
        mode: Mutex<Mode>,
    }

    impl CountingKeychain {
        fn reads(&self) -> usize {
            self.reads.load(Ordering::SeqCst)
        }

        fn set_mode(
            &self,
            mode: Mode,
        ) {
            *self.mode.lock().unwrap() = mode;
        }
    }

    impl Keychain for Arc<CountingKeychain> {
        fn read(
            &self,
            host_id: &str,
        ) -> Result<Option<String>, ReadError> {
            self.reads.fetch_add(1, Ordering::SeqCst);
            let mode = *self.mode.lock().unwrap();
            match mode {
                Mode::Denied => Err(ReadError {
                    message: "user denied".into(),
                    denied: true,
                }),
                Mode::Locked => Err(ReadError {
                    message: "keychain locked".into(),
                    denied: false,
                }),
                Mode::Normal => Ok(self.entries.lock().unwrap().get(host_id).cloned()),
            }
        }

        fn write(
            &self,
            host_id: &str,
            token: &str,
        ) -> Result<(), String> {
            let mut entries = self.entries.lock().unwrap();
            entries.insert(host_id.into(), token.into());
            Ok(())
        }

        fn remove(
            &self,
            host_id: &str,
        ) -> Result<(), String> {
            self.entries.lock().unwrap().remove(host_id);
            Ok(())
        }
    }

    type Fixture = (
        KeychainTokenStore<Arc<CountingKeychain>>,
        Arc<CountingKeychain>,
    );

    fn cached(entries: &[(&str, &str)]) -> Fixture {
        let keychain = Arc::new(CountingKeychain {
            entries: Mutex::default(),
            reads: AtomicUsize::new(0),
            mode: Mutex::new(Mode::Normal),
        });
        for (host, token) in entries {
            keychain.write(host, token).unwrap();
        }
        (KeychainTokenStore::new(Arc::clone(&keychain)), keychain)
    }

    fn assert_gets(
        entries: &[(&str, &str)],
        gets: &[&str],
        reads: usize,
    ) {
        let (store, keychain) = cached(entries);
        for host in gets {
            let expected = entries.iter().find(|(h, _)| h == host);
            let expected = expected.map(|(_, token)| (*token).to_owned());
            assert_eq!(store.get(host).unwrap(), expected, "{host}");
        }
        assert_eq!(keychain.reads(), reads, "{gets:?}");
    }

    #[test]
    fn repeated_gets_read_the_keychain_once_per_host() {
        type Case = (
            &'static [(&'static str, &'static str)],
            &'static [&'static str],
            usize,
        );
        let cases: &[Case] = &[
            (&[("a", "ta"), ("b", "tb")], &["a", "a", "a", "a", "a"], 1),
            (&[("a", "ta"), ("b", "tb")], &["a", "b", "b"], 2),
            (&[], &["a", "a"], 1),
        ];
        for (entries, gets, reads) in cases {
            assert_gets(entries, gets, *reads);
        }
    }

    #[test]
    fn set_updates_the_cache_without_a_read_and_delete_is_seen() {
        let (store, keychain) = cached(&[("a", "old")]);
        assert_eq!(store.get("a").unwrap(), Some("old".to_owned()));
        store.set("a", "new").unwrap();
        assert_eq!(store.get("a").unwrap(), Some("new".to_owned()));
        assert_eq!(keychain.reads(), 1);
        store.delete("a").unwrap();
        assert_eq!(store.get("a").unwrap(), None);
    }

    #[test]
    fn a_denied_read_stays_denied_until_the_host_is_paired_again() {
        let (store, keychain) = cached(&[("a", "tok")]);
        keychain.set_mode(Mode::Denied);
        for _ in 0..5 {
            assert_eq!(store.get("a"), Err("user denied".to_owned()));
        }
        assert_eq!(keychain.reads(), 1);
        store.set("a", "fresh").unwrap();
        assert_eq!(store.get("a").unwrap(), Some("fresh".to_owned()));
        assert_eq!(keychain.reads(), 1);
    }

    #[test]
    fn a_transient_failure_is_retried_on_the_next_get() {
        let (store, keychain) = cached(&[("a", "tok")]);
        keychain.set_mode(Mode::Locked);
        assert_eq!(store.get("a"), Err("keychain locked".to_owned()));
        keychain.set_mode(Mode::Normal);
        assert_eq!(store.get("a").unwrap(), Some("tok".to_owned()));
        assert_eq!(keychain.reads(), 2);
    }

    #[test]
    fn concurrent_first_gets_share_one_read() {
        let (store, keychain) = cached(&[("a", "tok")]);
        let read = || assert_eq!(store.get("a").unwrap(), Some("tok".to_owned()));
        std::thread::scope(|scope| {
            let joins: Vec<_> = (0..8).map(|_| scope.spawn(read)).collect();
            assert_eq!(joins.len(), 8);
        });
        assert_eq!(keychain.reads(), 1);
    }
}
