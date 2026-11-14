use std::sync::{Arc, Mutex};

use super::*;

#[derive(Default)]
struct Slot(Mutex<Option<String>>);

impl Keychain for Slot {
    fn read(&self, _host_id: &str) -> Result<Option<String>, ReadError> {
        Ok(self.0.lock().unwrap().clone())
    }

    fn write(&self, _host_id: &str, entry: &str) -> Result<(), Error> {
        *self.0.lock().unwrap() = Some(entry.to_owned());
        Ok(())
    }

    fn remove(&self, _host_id: &str) -> Result<(), Error> {
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
