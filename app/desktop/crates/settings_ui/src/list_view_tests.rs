use gpui::TestAppContext;

use crate::catalog::{AGENT_RECORDS, SESSION_RECORDS};
use crate::intent::Intent;
use crate::record_command::RecordCommand;
use crate::test_support::{open, run};

#[gpui::test]
fn retry_asks_the_backend_to_refresh_the_failed_list(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "active-sessions");
    run(&harness, Intent::Retry(SESSION_RECORDS.to_owned()), cx);
    assert_eq!(
        harness.backend.commands(),
        [(
            SESSION_RECORDS.to_owned(),
            String::new(),
            RecordCommand::Refresh
        )]
    );
}

#[gpui::test]
fn a_ready_list_and_its_sibling_states_render_on_one_page(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "providers");
    run(&harness, Intent::Retry(AGENT_RECORDS.to_owned()), cx);
    assert_eq!(harness.backend.commands().len(), 1);
}
