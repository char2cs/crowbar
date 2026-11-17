//! The settings side of the in-memory backend: reads, writes and subscriptions.

use std::rc::Rc;

use crate::error::Error;
use crate::fake_backend::FakeBackend;
use crate::key::Key;
use crate::settings_follow::{Listener, SettingsFollow};
use crate::settings_read::SettingsRead;
use crate::settings_write::SettingsWrite;
use crate::snapshot::Snapshot;
use crate::subscription::Subscription;
use crate::value::Value;

const REFUSAL_REASON: &str = "the fake backend is refusing writes";

fn refusal(key: &Key) -> Error {
    Error::Rejected {
        key: key.clone(),
        reason: REFUSAL_REASON.to_owned(),
    }
}

impl SettingsRead for FakeBackend {
    fn snapshot(&self) -> Snapshot {
        self.state.borrow().snapshot.clone()
    }

    fn get(
        &self,
        key: &Key,
    ) -> Option<Value> {
        self.state.borrow().snapshot.values.get(key).cloned()
    }
}

impl SettingsWrite for FakeBackend {
    fn set(
        &self,
        key: &Key,
        value: Value,
    ) -> Result<(), Error> {
        if self.state.borrow().refusing {
            return Err(refusal(key));
        }
        self.publish(key, value);
        Ok(())
    }

    fn invoke(
        &self,
        id: &str,
    ) -> Result<(), Error> {
        self.state.borrow_mut().invoked.push(id.to_owned());
        Ok(())
    }
}

impl SettingsFollow for FakeBackend {
    fn subscribe(
        &self,
        listener: Listener,
    ) -> Subscription {
        let id = {
            let mut state = self.state.borrow_mut();
            state.next_listener += 1;
            let id = state.next_listener;
            state.listeners.insert(id, listener);
            id
        };
        let state = Rc::clone(&self.state);
        Subscription::new(move || {
            state.borrow_mut().listeners.remove(&id);
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_refused_write_names_the_key() {
        let backend = FakeBackend::default();
        backend.refuse_writes();
        let key = Key::from("tabSize");
        let refused = backend.set(&key, Value::Bool(true));
        assert!(matches!(refused, Err(Error::Rejected { key: named, .. }) if named == key));
        assert_eq!(backend.snapshot(), Snapshot::default());
    }
}
