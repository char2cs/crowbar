//! An in-memory backend for tests and the gallery.

use std::cell::RefCell;
use std::rc::Rc;

use crate::change::Change;
use crate::fake_state::FakeState;
use crate::key::Key;
use crate::record_command::RecordCommand;
use crate::records::Records;
use crate::settings_follow::Listener;
use crate::snapshot::Snapshot;
use crate::value::Value;

/// A [`SettingsBackend`] and [`RecordBackend`] that keep everything in memory and log what they were asked to do.
#[derive(Clone, Default)]
pub struct FakeBackend {
    pub(crate) state: Rc<RefCell<FakeState>>,
}

impl FakeBackend {
    /// A backend that starts from `snapshot`.
    pub fn new(snapshot: Snapshot) -> Self {
        let state = FakeState {
            snapshot,
            ..FakeState::default()
        };
        Self {
            state: Rc::new(RefCell::new(state)),
        }
    }

    /// Serves `records` for the list `source`.
    #[must_use]
    pub fn with_records(
        self,
        source: &str,
        records: Records,
    ) -> Self {
        self.state
            .borrow_mut()
            .records
            .insert(source.to_owned(), records);
        self
    }

    /// Makes every following write fail until [`FakeBackend::accept_writes`].
    pub fn refuse_writes(&self) {
        self.state.borrow_mut().refusing = true;
    }

    /// Lets writes succeed again.
    pub fn accept_writes(&self) {
        self.state.borrow_mut().refusing = false;
    }

    /// The ids of the actions run so far, oldest first.
    pub fn invoked(&self) -> Vec<String> {
        self.state.borrow().invoked.clone()
    }

    /// The record commands applied so far as `(source, record, command)`.
    pub fn commands(&self) -> Vec<(String, String, RecordCommand)> {
        self.state.borrow().commands.clone()
    }

    /// Pretends another window changed `key`: stores it and tells every listener.
    pub fn publish(
        &self,
        key: &Key,
        value: Value,
    ) {
        self.state
            .borrow_mut()
            .snapshot
            .values
            .set(key.clone(), value.clone());
        self.notify(&Change::Setting(key.clone(), value));
    }

    pub(crate) fn notify(
        &self,
        change: &Change,
    ) {
        let listeners: Vec<Listener> = self.state.borrow().listeners.values().cloned().collect();
        for listener in listeners {
            listener(change);
        }
    }
}

impl std::fmt::Debug for FakeBackend {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("FakeBackend")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use super::*;
    use crate::error::Error;
    use crate::settings_follow::SettingsFollow as _;
    use crate::settings_read::SettingsRead as _;
    use crate::settings_write::SettingsWrite as _;

    #[test]
    fn writes_are_stored_announced_and_can_be_refused() {
        let backend = FakeBackend::default();
        let seen = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&seen);
        let subscription = backend.subscribe(Rc::new(move |change| {
            sink.borrow_mut().push(change.clone());
        }));
        let key = Key::from("tabSize");
        backend.set(&key, Value::Number(4.0)).unwrap();
        assert_eq!(backend.get(&key), Some(Value::Number(4.0)));
        assert_eq!(
            seen.borrow().as_slice(),
            [Change::Setting(key.clone(), Value::Number(4.0))]
        );
        backend.refuse_writes();
        assert!(matches!(
            backend.set(&key, Value::Number(8.0)),
            Err(Error::Rejected { .. })
        ));
        assert_eq!(
            backend.get(&key),
            Some(Value::Number(4.0)),
            "a refused write changes nothing"
        );
        backend.accept_writes();
        drop(subscription);
        backend.set(&key, Value::Number(2.0)).unwrap();
        assert_eq!(
            seen.borrow().len(),
            1,
            "a dropped subscription hears nothing"
        );
    }

    #[test]
    fn actions_are_logged_and_unknown_lists_are_loading() {
        let backend = FakeBackend::default();
        backend.invoke("hosts.add").unwrap();
        assert_eq!(backend.invoked(), ["hosts.add"]);
    }
}
