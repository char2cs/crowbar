use gpui::TestAppContext;

use crate::catalog::AGENT_RECORDS;
use crate::intent::Intent;
use crate::record_command::RecordCommand;
use crate::test_support::{open, run};

fn last(harness: &crate::test_support::Harness) -> (String, RecordCommand) {
    let (_, record, command) = harness.backend.commands().pop().unwrap();
    (record, command)
}

#[gpui::test]
fn flags_and_moves_reach_the_backend_as_record_commands(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "providers");
    let flag = Intent::Flag {
        source: AGENT_RECORDS.to_owned(),
        record: "codex".to_owned(),
        flag: "enabled".to_owned(),
        on: true,
    };
    run(&harness, flag, cx);
    assert_eq!(
        last(&harness),
        (
            "codex".to_owned(),
            RecordCommand::SetFlag {
                flag: "enabled".to_owned(),
                on: true
            }
        )
    );
    let up = Intent::Reorder {
        source: AGENT_RECORDS.to_owned(),
        record: "codex".to_owned(),
        to: 0,
    };
    run(&harness, up, cx);
    assert_eq!(
        last(&harness),
        ("codex".to_owned(), RecordCommand::Move { to: 0 })
    );
}
