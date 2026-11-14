use crate::error::Error;
use crate::keychain::Keychain;
use crate::read_error::ReadError;

const SERVICE: &str = "crowbar-desktop";

/// Raw access to the OS keychain under the `crowbar-desktop` service.
#[derive(Debug, Clone, Copy)]
pub struct OsKeychain;

fn is_denial(error: &keyring::Error) -> bool {
    match error {
        keyring::Error::NoStorageAccess(_) => false,
        #[cfg(target_os = "macos")]
        keyring::Error::PlatformFailure(source) => source
            .downcast_ref::<security_framework::base::Error>()
            .is_none_or(|e| !is_transient_status(e.code())),
        #[cfg(not(target_os = "macos"))]
        keyring::Error::PlatformFailure(_) => false,
        _ => true,
    }
}

#[cfg(target_os = "macos")]
fn is_transient_status(code: i32) -> bool {
    matches!(code, -25308 | -25291 | -25292 | -25294 | -25295)
}

fn read_error(error: &keyring::Error) -> ReadError {
    ReadError {
        message: error.to_string(),
        denied: is_denial(error),
    }
}

fn store_error(error: &keyring::Error) -> Error {
    Error::Store(error.to_string())
}

impl Keychain for OsKeychain {
    fn read(&self, host_id: &str) -> Result<Option<String>, ReadError> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| read_error(&e))?;
        match entry.get_password() {
            Ok(secret) => Ok(Some(secret)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(read_error(&e)),
        }
    }

    fn write(&self, host_id: &str, entry: &str) -> Result<(), Error> {
        let item = keyring::Entry::new(SERVICE, host_id).map_err(|e| store_error(&e))?;
        item.set_password(entry).map_err(|e| store_error(&e))
    }

    fn remove(&self, host_id: &str) -> Result<(), Error> {
        let item = keyring::Entry::new(SERVICE, host_id).map_err(|e| store_error(&e))?;
        match item.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(e) => Err(store_error(&e)),
        }
    }
}

#[cfg(test)]
#[path = "os_keychain_tests.rs"]
mod tests;
