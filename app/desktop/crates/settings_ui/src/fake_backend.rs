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

const REFUSAL_REASON: &str = "the fake backend is refusing writes";

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

    fn notify(
        &self,
        change: &Change,
    ) {
        let listeners: Vec<Listener> = self.state.borrow().listeners.values().cloned().collect();
        for listener in listeners {
            listener(change);
        }
    }

    fn edit_records(
        &self,
        source: &str,
        edit: impl FnOnce(&mut Vec<crate::record::Record>),
    ) {
        if let Some(Records::Ready(list)) = self.state.borrow_mut().records.get_mut(source) {
            edit(list);
        }
        self.notify(&Change::Records(source.to_owned()));
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

fn refusal(key: &Key) -> Error {
    Error::Rejected {
        key: key.clone(),
        reason: REFUSAL_REASON.to_owned(),
    }
}

impl SettingsBackend for FakeBackend {
    fn snapshot(&self) -> Snapshot {
        self.state.borrow().snapshot.clone()
    }

    fn get(
        &self,
        key: &Key,
    ) -> Option<Value> {
        self.state.borrow().snapshot.values.get(key).cloned()
    }

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

fn move_record(
    list: &mut Vec<crate::record::Record>,
    id: &str,
    to: usize,
) {
    let Some(from) = list.iter().position(|entry| entry.id == id) else {
        return;
    };
    let entry = list.remove(from);
    list.insert(to.min(list.len()), entry);
}

#[cfg(test)]
mod tests {
    use crate::test_support::{Case, run_cases};
    use std::cell::RefCell;

    use super::*;
    use crate::record::Record;
    use crate::record_flag::RecordFlag;

    fn backend() -> FakeBackend {
        let list = vec![
            Record::new("a", "A").flag(RecordFlag::new("enabled", "Enable").on()),
            Record::new("b", "B"),
            Record::new("c", "C"),
        ];
        FakeBackend::new(Snapshot::default()).with_records("agents", Records::Ready(list))
    }

    fn ids(backend: &FakeBackend) -> Vec<String> {
        match backend.records("agents") {
            Records::Ready(list) => list.iter().map(|record| record.id.clone()).collect(),
            other => panic!("expected a ready list, got {other:?}"),
        }
    }

    fn writes_are_stored_announced_and_can_be_refused() {
        let backend = backend();
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

    fn record_commands_edit_the_list_and_are_logged() {
        let backend = backend();
        backend
            .apply(
                "agents",
                "a",
                RecordCommand::SetFlag {
                    flag: "enabled".to_owned(),
                    on: false,
                },
            )
            .unwrap();
        let Records::Ready(list) = backend.records("agents") else {
            panic!("list is ready");
        };
        assert!(
            list.first()
                .is_some_and(|record| record.flags.iter().all(|flag| !flag.on))
        );
        backend
            .apply("agents", "c", RecordCommand::Move { to: 0 })
            .unwrap();
        assert_eq!(ids(&backend), ["c", "a", "b"]);
        backend
            .apply(
                "agents",
                "b",
                RecordCommand::Run {
                    action: "kill".to_owned(),
                },
            )
            .unwrap();
        assert_eq!(ids(&backend), ["c", "a"]);
        assert_eq!(backend.commands().len(), 3);
    }

    fn actions_are_logged_and_unknown_lists_are_loading() {
        let backend = backend();
        backend.invoke("hosts.add").unwrap();
        assert_eq!(backend.invoked(), ["hosts.add"]);
        assert_eq!(backend.records("nope"), Records::Loading);
    }

    const CASES: &[Case] = &[
        (
            "writes are stored announced and can be refused",
            writes_are_stored_announced_and_can_be_refused,
        ),
        (
            "record commands edit the list and are logged",
            record_commands_edit_the_list_and_are_logged,
        ),
        (
            "actions are logged and unknown lists are loading",
            actions_are_logged_and_unknown_lists_are_loading,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
