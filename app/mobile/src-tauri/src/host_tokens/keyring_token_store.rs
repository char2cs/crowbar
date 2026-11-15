use super::keychain_token_store::KeychainTokenStore;
use super::os_keychain::OsKeychain;
use super::token_store::TokenStore;

const SERVICE: &str = "crowbar-desktop";

pub(crate) struct KeyringTokenStore(KeychainTokenStore<OsKeychain>);

impl Default for KeyringTokenStore {
    fn default() -> Self {
        Self(KeychainTokenStore::new(OsKeychain::new(SERVICE)))
    }
}

impl TokenStore for KeyringTokenStore {
    fn get(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, String> {
        self.0.get(host_id)
    }

    fn set(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), String> {
        self.0.set(host_id, token)
    }

    fn delete(
        &self,
        host_id: &str,
    ) -> Result<(), String> {
        self.0.delete(host_id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "needs the OS keychain"]
    fn the_production_store_roundtrips_against_the_real_keychain() {
        let store = KeyringTokenStore::default();
        let host_id = "crowbar-desktop-test-task3-keychain-roundtrip";
        store.delete(host_id).unwrap();
        assert_eq!(store.get(host_id).unwrap(), None);
        store.set(host_id, "sometoken").unwrap();
        assert_eq!(store.get(host_id).unwrap(), Some("sometoken".to_owned()));
        store.delete(host_id).unwrap();
        assert_eq!(store.get(host_id).unwrap(), None);
    }
}
