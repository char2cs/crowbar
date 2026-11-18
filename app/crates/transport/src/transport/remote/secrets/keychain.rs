use crate::error::Error;
use crate::transport::remote::secrets::read_error::ReadError;

/// Raw keychain access, split from the cache in front of it so the caching can be tested without
/// touching an OS keychain.
pub trait Keychain: Send + Sync {
    /// Reads one entry.
    ///
    /// # Errors
    /// A [`ReadError`] saying whether the user refused.
    fn read(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, ReadError>;

    /// Writes one entry.
    ///
    /// # Errors
    /// [`Error::Store`] when the keychain refuses the write.
    fn write(
        &self,
        host_id: &str,
        entry: &str,
    ) -> Result<(), Error>;

    /// Removes one entry; a missing one is not an error.
    ///
    /// # Errors
    /// [`Error::Store`] when the keychain refuses the removal.
    fn remove(
        &self,
        host_id: &str,
    ) -> Result<(), Error>;
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use super::*;

    #[derive(Default)]
    struct Slot(Mutex<Option<String>>);

    impl Keychain for Slot {
        fn read(
            &self,
            _host_id: &str,
        ) -> Result<Option<String>, ReadError> {
            Ok(self.0.lock().unwrap().clone())
        }

        fn write(
            &self,
            _host_id: &str,
            entry: &str,
        ) -> Result<(), Error> {
            *self.0.lock().unwrap() = Some(entry.to_owned());
            Ok(())
        }

        fn remove(
            &self,
            _host_id: &str,
        ) -> Result<(), Error> {
            *self.0.lock().unwrap() = None;
            Ok(())
        }
    }

    #[test]
    fn a_keychain_is_usable_as_a_shared_trait_object() {
        let keychain: Arc<dyn Keychain> = Arc::new(Slot::default());
        keychain.write("h", "entry").unwrap();
        assert_eq!(keychain.read("h").unwrap().as_deref(), Some("entry"));
        keychain.remove("h").unwrap();
        assert_eq!(keychain.read("h").unwrap(), None);
    }
}
