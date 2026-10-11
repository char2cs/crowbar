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
