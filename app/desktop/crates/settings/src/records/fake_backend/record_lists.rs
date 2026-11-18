//! The record side of the in-memory backend: lists, commands and their effect on the lists.

use crate::records::Records;
use crate::records::fake_backend::FakeBackend;
use crate::records::record::Record;
use crate::records::record_backend::RecordBackend;
use crate::records::record_command::RecordCommand;
use crate::records::record_flag::RecordFlag;
use crate::values::Change;
use crate::values::Error;

impl FakeBackend {
    pub(crate) fn edit_records(
        &self,
        source: &str,
        edit: impl FnOnce(&mut Vec<Record>),
    ) {
        if let Some(Records::Ready(list)) = self.state.borrow_mut().records.get_mut(source) {
            edit(list);
        }
        self.notify(&Change::Records(source.to_owned()));
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
    list: &'a mut [Record],
    id: &str,
    flag: &str,
) -> Option<&'a mut RecordFlag> {
    list.iter_mut()
        .filter(|entry| entry.id == id)
        .flat_map(|entry| entry.flags.iter_mut())
        .find(|candidate| candidate.id == flag)
}

fn move_record(
    list: &mut Vec<Record>,
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
    use super::*;
    use crate::values::Snapshot;

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

    #[test]
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

    #[test]
    fn unknown_lists_are_loading() {
        assert_eq!(backend().records("nope"), Records::Loading);
    }
}
