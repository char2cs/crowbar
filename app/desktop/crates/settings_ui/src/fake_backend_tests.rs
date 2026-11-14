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

#[test]
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
fn actions_are_logged_and_unknown_lists_are_loading() {
    let backend = backend();
    backend.invoke("hosts.add").unwrap();
    assert_eq!(backend.invoked(), ["hosts.add"]);
    assert_eq!(backend.records("nope"), Records::Loading);
}
