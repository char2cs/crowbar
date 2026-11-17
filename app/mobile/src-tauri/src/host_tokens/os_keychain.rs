use super::keychain::Keychain;
use super::read_error::ReadError;
use crate::token_error::TokenError;

pub(super) struct OsKeychain {
    service: &'static str,
}

impl OsKeychain {
    pub(super) fn new(service: &'static str) -> Self {
        Self { service }
    }

    fn entry(
        &self,
        host_id: &str,
    ) -> Result<keyring::Entry, keyring::Error> {
        keyring::Entry::new(self.service, host_id)
    }
}

impl Keychain for OsKeychain {
    fn read(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, ReadError> {
        let entry = self.entry(host_id).map_err(ReadError::from_keyring)?;
        match entry.get_password() {
            Ok(token) => Ok(Some(token)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(ReadError::from_keyring(e)),
        }
    }

    fn write(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), TokenError> {
        let entry = self.entry(host_id).map_err(TokenError::new)?;
        entry.set_password(token).map_err(TokenError::new)
    }

    fn remove(
        &self,
        host_id: &str,
    ) -> Result<(), TokenError> {
        let entry = self.entry(host_id).map_err(TokenError::new)?;
        match entry.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(e) => Err(TokenError::new(e)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "needs the OS keychain"]
    fn the_real_keychain_roundtrips_a_token() {
        let keychain = OsKeychain::new("crowbar-desktop-test-keychain-roundtrip");
        let host_id = "roundtrip-host";
        keychain.remove(host_id).unwrap();
        assert_eq!(keychain.read(host_id).ok(), Some(None));
        keychain.write(host_id, "sometoken").unwrap();
        assert_eq!(
            keychain.read(host_id).ok(),
            Some(Some("sometoken".to_owned()))
        );
        keychain.remove(host_id).unwrap();
        assert_eq!(keychain.read(host_id).ok(), Some(None));
    }
}
