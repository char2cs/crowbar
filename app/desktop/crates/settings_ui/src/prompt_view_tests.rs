use gpui::TestAppContext;

use crate::control::Control;
use crate::intent::Intent;
use crate::test_support::{inspect, open, row, run};

fn clear_data() -> crate::button_spec::ButtonSpec {
    let Control::Button(spec) = row("browserClearData").control else {
        panic!("browserClearData is a button row");
    };
    spec
}

#[gpui::test]
fn a_destructive_button_asks_and_only_the_accept_runs_it(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "browser");
    run(&harness, Intent::Button(clear_data()), cx);
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .prompt()
        .is_some()));
    run(&harness, Intent::Dismiss, cx);
    assert_eq!(harness.backend.invoked().len(), 0);
    run(&harness, Intent::Button(clear_data()), cx);
    run(&harness, Intent::Accept, cx);
    assert_eq!(harness.backend.invoked().len(), 1);
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .prompt()
        .is_none()));
}
