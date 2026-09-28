//! Bearer tokens for remote hosts, in the OS keychain — never in the
//! tauri-plugin-store config JSON alongside a host's name/endpoint/
//! fingerprint (that store is client-visible app config; a bearer token is a
//! secret).

use tauri::State;

/// The seam host_tokens' Tauri commands (and, eventually, api_proxy's proxy
/// path) go through: production talks to the OS keychain; tests use an
/// in-memory fake, since exercising the real keychain in CI is unreliable
/// (prompts, platform differences) and unnecessary to prove this module's
/// own logic.
pub trait TokenStore: Send + Sync {
    #[allow(dead_code)] // dead_code: consumed by Task 5's forward_remote, which reads the stored token before dialing the remote host
    fn get(&self, host_id: &str) -> Result<Option<String>, String>;
    fn set(&self, host_id: &str, token: &str) -> Result<(), String>;
    fn delete(&self, host_id: &str) -> Result<(), String>;
}

const SERVICE: &str = "crowbar-desktop";

pub struct KeyringTokenStore;

impl TokenStore for KeyringTokenStore {
    fn get(&self, host_id: &str) -> Result<Option<String>, String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        match entry.get_password() {
            Ok(token) => Ok(Some(token)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(e.to_string()),
        }
    }

    fn set(&self, host_id: &str, token: &str) -> Result<(), String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        entry.set_password(token).map_err(|e| e.to_string())
    }

    fn delete(&self, host_id: &str) -> Result<(), String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        match entry.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(e) => Err(e.to_string()),
        }
    }
}

#[tauri::command]
pub fn store_host_token(
    host_id: String,
    token: String,
    store: State<'_, Box<dyn TokenStore>>,
) -> Result<(), String> {
    store.set(&host_id, &token)
}

#[tauri::command]
pub fn forget_host_token(
    host_id: String,
    store: State<'_, Box<dyn TokenStore>>,
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

    /// Exercises the REAL OS keychain, not the fake — `keyring = "3"` has no
    /// default backend feature and silently falls back to an in-memory mock
    /// if the platform-native feature is missing from Cargo.toml, so a
    /// round-trip against `FakeStore` alone cannot catch that. `#[ignore]`d
    /// because it prompts/differs across CI runners; run manually with
    /// `cargo test host_tokens -- --ignored`.
    #[test]
    #[ignore]
    fn keyring_token_store_roundtrips_against_the_real_os_keychain() {
        let store = KeyringTokenStore;
        let host_id = "crowbar-desktop-test-task3-keychain-roundtrip";

        store.delete(host_id).unwrap(); // clean slate from any prior failed run
        assert_eq!(store.get(host_id).unwrap(), None);

        store.set(host_id, "sometoken").unwrap();
        assert_eq!(store.get(host_id).unwrap(), Some("sometoken".to_string()));

        store.delete(host_id).unwrap();
        assert_eq!(store.get(host_id).unwrap(), None);
    }
}
