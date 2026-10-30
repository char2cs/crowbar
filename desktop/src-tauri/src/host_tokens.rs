//! Bearer tokens for remote hosts, in the OS keychain — never in the
//! tauri-plugin-store config JSON alongside a host's name/endpoint/
//! fingerprint (that store is client-visible app config; a bearer token is a
//! secret).

use std::sync::Arc;

use tauri::State;

use crate::tls_pin::{hex, CertificateDigest};

/// What pairing stores for a host: its token, and the certificate that token
/// may be sent to. Kept in one keychain entry so nothing the webview or the
/// config can write moves the token to another identity.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Credential {
    pub token: String,
    pub pin: CertificateDigest,
}

impl Credential {
    pub fn encode(&self) -> String {
        serde_json::json!({ "token": self.token, "pin": hex(&self.pin) }).to_string()
    }

    /// `None` for anything pairing did not write, including a blank token.
    pub fn decode(stored: &str) -> Option<Self> {
        let value: serde_json::Value = serde_json::from_str(stored).ok()?;
        let token = value["token"].as_str().filter(|t| !t.trim().is_empty())?;
        Some(Self {
            token: token.to_string(),
            pin: parse_digest(value["pin"].as_str()?)?,
        })
    }
}

fn parse_digest(text: &str) -> Option<CertificateDigest> {
    let mut digest = CertificateDigest::default();
    if text.len() != digest.len() * 2 || !text.is_ascii() {
        return None;
    }
    for (byte, pair) in digest.iter_mut().zip(text.as_bytes().chunks(2)) {
        *byte = u8::from_str_radix(std::str::from_utf8(pair).ok()?, 16).ok()?;
    }
    Some(digest)
}

/// The seam host_tokens' Tauri commands (and api_proxy's remote path) go
/// through: production talks to the OS keychain; tests use an
/// in-memory fake, since exercising the real keychain in CI is unreliable
/// (prompts, platform differences) and unnecessary to prove this module's
/// own logic. Entries are [`Credential::encode`]d.
pub trait TokenStore: Send + Sync {
    fn get(&self, host_id: &str) -> Result<Option<String>, String>;
    fn set(&self, host_id: &str, token: &str) -> Result<(), String>;
    fn delete(&self, host_id: &str) -> Result<(), String>;
}

const SERVICE: &str = "crowbar-desktop";

/// Raw keychain access, split from [`KeychainTokenStore`] so the caching in
/// front of it can be tested without touching the OS keychain.
trait Keychain: Send + Sync {
    fn read(&self, host_id: &str) -> Result<Option<String>, String>;
    fn write(&self, host_id: &str, token: &str) -> Result<(), String>;
    fn remove(&self, host_id: &str) -> Result<(), String>;
}

struct OsKeychain;

impl Keychain for OsKeychain {
    fn read(&self, host_id: &str) -> Result<Option<String>, String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        match entry.get_password() {
            Ok(token) => Ok(Some(token)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(e.to_string()),
        }
    }

    fn write(&self, host_id: &str, token: &str) -> Result<(), String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        entry.set_password(token).map_err(|e| e.to_string())
    }

    fn remove(&self, host_id: &str) -> Result<(), String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        match entry.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(e) => Err(e.to_string()),
        }
    }
}

/// Reads each host's entry from the keychain at most once per process: every
/// keychain read can raise its own macOS access prompt, and callers read on
/// every request and socket open. A failed or denied read is remembered too,
/// or each caller would re-prompt forever; pairing the host again clears it.
/// The lock is held across a read so concurrent first reads prompt once.
struct KeychainTokenStore<K: Keychain> {
    keychain: K,
    cache: std::sync::Mutex<std::collections::HashMap<String, Result<Option<String>, String>>>,
}

impl<K: Keychain> KeychainTokenStore<K> {
    fn new(keychain: K) -> Self {
        Self {
            keychain,
            cache: Default::default(),
        }
    }
}

impl<K: Keychain> TokenStore for KeychainTokenStore<K> {
    fn get(&self, host_id: &str) -> Result<Option<String>, String> {
        let mut cache = self.cache.lock().unwrap_or_else(|e| e.into_inner());
        if let Some(cached) = cache.get(host_id) {
            return cached.clone();
        }
        let stored = self.keychain.read(host_id);
        cache.insert(host_id.to_string(), stored.clone());
        stored
    }

    fn set(&self, host_id: &str, token: &str) -> Result<(), String> {
        let mut cache = self.cache.lock().unwrap_or_else(|e| e.into_inner());
        self.keychain.write(host_id, token)?;
        cache.insert(host_id.to_string(), Ok(Some(token.to_string())));
        Ok(())
    }

    fn delete(&self, host_id: &str) -> Result<(), String> {
        let mut cache = self.cache.lock().unwrap_or_else(|e| e.into_inner());
        self.keychain.remove(host_id)?;
        cache.insert(host_id.to_string(), Ok(None));
        Ok(())
    }
}

/// The production [`TokenStore`]: the OS keychain behind a per-process cache.
pub struct KeyringTokenStore(KeychainTokenStore<OsKeychain>);

impl KeyringTokenStore {
    pub fn new() -> Self {
        Self(KeychainTokenStore::new(OsKeychain))
    }
}

impl TokenStore for KeyringTokenStore {
    fn get(&self, host_id: &str) -> Result<Option<String>, String> {
        self.0.get(host_id)
    }

    fn set(&self, host_id: &str, token: &str) -> Result<(), String> {
        self.0.set(host_id, token)
    }

    fn delete(&self, host_id: &str) -> Result<(), String> {
        self.0.delete(host_id)
    }
}

#[tauri::command]
pub fn forget_host_token(
    host_id: String,
    store: State<'_, Arc<dyn TokenStore>>,
) -> Result<(), String> {
    store.delete(&host_id)
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::sync::Mutex;

    use super::*;

    struct FakeStore(Mutex<HashMap<String, String>>);
    impl FakeStore {
        fn new() -> Self {
            Self(Mutex::new(HashMap::new()))
        }
    }
    impl TokenStore for FakeStore {
        fn get(&self, host_id: &str) -> Result<Option<String>, String> {
            Ok(self.0.lock().unwrap().get(host_id).cloned())
        }
        fn set(&self, host_id: &str, token: &str) -> Result<(), String> {
            self.0
                .lock()
                .unwrap()
                .insert(host_id.to_string(), token.to_string());
            Ok(())
        }
        fn delete(&self, host_id: &str) -> Result<(), String> {
            self.0.lock().unwrap().remove(host_id);
            Ok(())
        }
    }

    #[derive(Default)]
    struct CountingKeychain {
        entries: Mutex<HashMap<String, String>>,
        reads: std::sync::atomic::AtomicUsize,
        fail_reads: std::sync::atomic::AtomicBool,
    }
    impl CountingKeychain {
        fn reads(&self) -> usize {
            self.reads.load(std::sync::atomic::Ordering::SeqCst)
        }
    }
    impl Keychain for Arc<CountingKeychain> {
        fn read(&self, host_id: &str) -> Result<Option<String>, String> {
            self.reads.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            if self.fail_reads.load(std::sync::atomic::Ordering::SeqCst) {
                return Err("user denied".into());
            }
            Ok(self.entries.lock().unwrap().get(host_id).cloned())
        }
        fn write(&self, host_id: &str, token: &str) -> Result<(), String> {
            self.entries
                .lock()
                .unwrap()
                .insert(host_id.into(), token.into());
            Ok(())
        }
        fn remove(&self, host_id: &str) -> Result<(), String> {
            self.entries.lock().unwrap().remove(host_id);
            Ok(())
        }
    }

    fn cached(
        entries: &[(&str, &str)],
    ) -> (
        KeychainTokenStore<Arc<CountingKeychain>>,
        Arc<CountingKeychain>,
    ) {
        let keychain = Arc::new(CountingKeychain::default());
        for (host, token) in entries {
            keychain.write(host, token).unwrap();
        }
        (KeychainTokenStore::new(keychain.clone()), keychain)
    }

    #[test]
    fn repeated_gets_read_the_keychain_once_per_host() {
        let (store, keychain) = cached(&[("a", "ta"), ("b", "tb")]);
        for _ in 0..5 {
            assert_eq!(store.get("a").unwrap(), Some("ta".to_string()));
        }
        assert_eq!(keychain.reads(), 1);
        assert_eq!(store.get("b").unwrap(), Some("tb".to_string()));
        assert_eq!(store.get("b").unwrap(), Some("tb".to_string()));
        assert_eq!(keychain.reads(), 2);
    }

    #[test]
    fn a_missing_entry_is_cached_too() {
        let (store, keychain) = cached(&[]);
        assert_eq!(store.get("a").unwrap(), None);
        assert_eq!(store.get("a").unwrap(), None);
        assert_eq!(keychain.reads(), 1);
    }

    #[test]
    fn set_updates_the_cache_without_a_read() {
        let (store, keychain) = cached(&[("a", "old")]);
        assert_eq!(store.get("a").unwrap(), Some("old".to_string()));
        store.set("a", "new").unwrap();
        assert_eq!(store.get("a").unwrap(), Some("new".to_string()));
        assert_eq!(keychain.reads(), 1);
    }

    #[test]
    fn delete_is_seen_by_the_next_get() {
        let (store, _) = cached(&[("a", "tok")]);
        assert_eq!(store.get("a").unwrap(), Some("tok".to_string()));
        store.delete("a").unwrap();
        assert_eq!(store.get("a").unwrap(), None);
    }

    #[test]
    fn a_denied_read_prompts_once_then_stays_denied_until_the_host_is_paired_again() {
        let (store, keychain) = cached(&[("a", "tok")]);
        keychain
            .fail_reads
            .store(true, std::sync::atomic::Ordering::SeqCst);
        for _ in 0..5 {
            assert_eq!(store.get("a"), Err("user denied".to_string()));
        }
        assert_eq!(keychain.reads(), 1);

        store.set("a", "fresh").unwrap();
        assert_eq!(store.get("a").unwrap(), Some("fresh".to_string()));
        assert_eq!(keychain.reads(), 1);
    }

    #[test]
    fn concurrent_first_gets_share_one_read() {
        let (store, keychain) = cached(&[("a", "tok")]);
        std::thread::scope(|scope| {
            for _ in 0..8 {
                scope.spawn(|| assert_eq!(store.get("a").unwrap(), Some("tok".to_string())));
            }
        });
        assert_eq!(keychain.reads(), 1);
    }

    #[test]
    fn set_then_get_roundtrips() {
        let store = FakeStore::new();
        store.set("host-1", "sometoken").unwrap();
        assert_eq!(store.get("host-1").unwrap(), Some("sometoken".to_string()));
    }

    #[test]
    fn get_missing_host_returns_none() {
        let store = FakeStore::new();
        assert_eq!(store.get("no-such-host").unwrap(), None);
    }

    #[test]
    fn delete_removes_it() {
        let store = FakeStore::new();
        store.set("host-1", "tok").unwrap();
        store.delete("host-1").unwrap();
        assert_eq!(store.get("host-1").unwrap(), None);
    }

    #[test]
    fn a_credential_reads_back_as_written_and_nothing_else_reads_as_one() {
        let credential = Credential {
            token: "sometoken".into(),
            pin: std::array::from_fn(|i| i as u8 * 7),
        };
        assert_eq!(
            Credential::decode(&credential.encode()),
            Some(credential.clone())
        );

        let pin = crate::tls_pin::hex(&credential.pin);
        let not_credentials = [
            "sometoken".to_string(),
            serde_json::json!({ "token": "sometoken" }).to_string(),
            serde_json::json!({ "token": " ", "pin": pin }).to_string(),
            serde_json::json!({ "token": "sometoken", "pin": &pin[2..] }).to_string(),
            serde_json::json!({ "token": "sometoken", "pin": pin.replace('0', "g") }).to_string(),
        ];
        for stored in not_credentials {
            assert_eq!(Credential::decode(&stored), None, "{stored}");
        }
    }

    /// Exercises the REAL OS keychain, not the fake — `keyring = "3"` has no
    /// default backend feature and silently falls back to an in-memory mock
    /// if the platform-native feature is missing from Cargo.toml, so a
    /// round-trip against `FakeStore` alone cannot catch that. `#[ignore]`d
    /// because it prompts/differs across CI runners; run manually with
    /// `cargo test host_tokens -- --ignored`.
    #[test]
    #[ignore]
    fn keyring_token_store_roundtrips_against_the_real_os_keychain() {
        let store = KeyringTokenStore::new();
        let host_id = "crowbar-desktop-test-task3-keychain-roundtrip";

        store.delete(host_id).unwrap(); // clean slate from any prior failed run
        assert_eq!(store.get(host_id).unwrap(), None);

        store.set(host_id, "sometoken").unwrap();
        assert_eq!(store.get(host_id).unwrap(), Some("sometoken".to_string()));

        store.delete(host_id).unwrap();
        assert_eq!(store.get(host_id).unwrap(), None);
    }
}
