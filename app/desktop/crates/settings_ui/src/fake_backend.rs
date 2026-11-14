//! An in-memory backend for tests and the gallery.

use std::cell::RefCell;
use std::rc::Rc;

use crate::change::Change;
use crate::error::Error;
use crate::fake_state::FakeState;
use crate::key::Key;
use crate::record_backend::RecordBackend;
use crate::record_command::RecordCommand;
use crate::records::Records;
use crate::settings_backend::{Listener, SettingsBackend};
use crate::snapshot::Snapshot;
use crate::subscription::Subscription;
use crate::value::Value;

/// A [`SettingsBackend`] and [`RecordBackend`] that keep everything in memory and log what they were asked to do.
#[derive(Clone, Default)]
pub struct FakeBackend {
    state: Rc<RefCell<FakeState>>,
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
    pub fn with_records(self, source: &str, records: Records) -> Self {
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
    pub fn publish(&self, key: &Key, value: Value) {
        self.state
            .borrow_mut()
            .snapshot
            .values
            .set(key.clone(), value.clone());
        self.notify(&Change::Setting(key.clone(), value));
    }

    fn notify(&self, change: &Change) {
        let listeners: Vec<Listener> = self.state.borrow().listeners.values().cloned().collect();
        for listener in listeners {
            listener(change);
        }
    }

    fn edit_records(&self, source: &str, edit: impl FnOnce(&mut Vec<crate::record::Record>)) {
        if let Some(Records::Ready(list)) = self.state.borrow_mut().records.get_mut(source) {
            edit(list);
        }
        self.notify(&Change::Records(source.to_owned()));
    }
}

impl std::fmt::Debug for FakeBackend {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("FakeBackend")
            .finish_non_exhaustive()
    }
}

impl SettingsBackend for FakeBackend {
    fn snapshot(&self) -> Snapshot {
        self.state.borrow().snapshot.clone()
    }

    fn get(&self, key: &Key) -> Option<Value> {
        self.state.borrow().snapshot.values.get(key).cloned()
    }

    fn set(&self, key: &Key, value: Value) -> Result<(), Error> {
        if self.state.borrow().refusing {
            return Err(Error::Rejected {
                key: key.clone(),
                reason: "the fake backend is refusing writes".to_owned(),
            });
        }
        self.publish(key, value);
        Ok(())
    }

    fn invoke(&self, id: &str) -> Result<(), Error> {
        self.state.borrow_mut().invoked.push(id.to_owned());
        Ok(())
    }

    fn subscribe(&self, listener: Listener) -> Subscription {
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

impl RecordBackend for FakeBackend {
    fn records(&self, source: &str) -> Records {
        self.state
            .borrow()
            .records
            .get(source)
            .cloned()
            .unwrap_or(Records::Loading)
    }

    fn apply(&self, source: &str, record: &str, command: RecordCommand) -> Result<(), Error> {
        self.state.borrow_mut().commands.push((
            source.to_owned(),
            record.to_owned(),
            command.clone(),
        ));
        let id = record.to_owned();
        self.edit_records(source, |list| match command {
            RecordCommand::SetFlag { flag, on } => flag_slot(list, &id, &flag)
                .into_iter()
                .for_each(|slot| slot.on = on),
            RecordCommand::Move { to } => move_record(list, &id, to),
            RecordCommand::Run { .. } => list.retain(|entry| entry.id != id),
            RecordCommand::Refresh => {},
        });
        Ok(())
    }
}

fn flag_slot<'a>(
    list: &'a mut [crate::record::Record],
    id: &str,
    flag: &str,
) -> Option<&'a mut crate::record_flag::RecordFlag> {
    list.iter_mut()
        .filter(|entry| entry.id == id)
        .flat_map(|entry| entry.flags.iter_mut())
        .find(|candidate| candidate.id == flag)
}

fn move_record(list: &mut Vec<crate::record::Record>, id: &str, to: usize) {
    let Some(from) = list.iter().position(|entry| entry.id == id) else {
        return;
    };
    let entry = list.remove(from);
    list.insert(to.min(list.len()), entry);
}

#[cfg(test)]
#[path = "fake_backend_tests.rs"]
mod tests;
