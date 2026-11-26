//! An in-memory backend for tests.

mod state;
mod values;

use std::cell::RefCell;
use std::rc::Rc;

use crate::records::Records;
use crate::records::fake_backend::state::State;
use crate::records::record_backend::RecordBackend;
use crate::records::record_command::RecordCommand;
use crate::values::Change;
use crate::values::Error;
use crate::values::Key;
use crate::values::Listener;
use crate::values::Snapshot;
use crate::values::Value;

/// A [`Backend`](crate::values::Backend) and [`RecordBackend`](crate::records::record_backend::RecordBackend)
/// that keep everything in memory and log what they were asked to do.
#[derive(Clone, Default)]
pub struct FakeBackend {
    pub(crate) state: Rc<RefCell<State>>,
}

impl FakeBackend {
    /// A backend that starts from `snapshot`.
    pub fn new(snapshot: Snapshot) -> Self {
        let state = State {
            snapshot,
            ..State::default()
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

    /// Makes every following write fail.
    pub fn refuse_writes(&self) {
        self.state.borrow_mut().refusing = true;
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

impl RecordBackend for FakeBackend {
    fn records(
        &self,
        source: &str,
    ) -> Records {
        self.state
            .borrow()
            .records
            .get(source)
            .cloned()
            .unwrap_or(Records::Loading)
    }

    fn apply(
        &self,
        source: &str,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error> {
        let logged = (source.to_owned(), record.to_owned(), command);
        self.state.borrow_mut().commands.push(logged);
        self.notify(&Change::Records(source.to_owned()));
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::values::Write as _;

    #[test]
    fn actions_and_record_commands_are_logged_in_order() {
        let backend = FakeBackend::default();
        backend.invoke("hosts.add").unwrap();
        backend
            .apply("hosts", "box", RecordCommand::Refresh)
            .unwrap();
        assert_eq!(backend.invoked(), ["hosts.add"]);
        let logged = ("hosts".to_owned(), "box".to_owned(), RecordCommand::Refresh);
        assert_eq!(backend.commands(), [logged]);
    }
}
