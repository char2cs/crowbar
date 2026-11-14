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
    fn read(&self, host_id: &str) -> Result<Option<String>, ReadError> {
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

    fn write(&self, host_id: &str, entry: &str) -> Result<(), Error> {
        self.entries
            .lock()
            .unwrap()
            .insert(host_id.into(), entry.into());
        Ok(())
    }

    fn remove(&self, host_id: &str) -> Result<(), Error> {
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
