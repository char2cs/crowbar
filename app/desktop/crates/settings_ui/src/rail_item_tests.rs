use gpui::TestAppContext;

use crate::intent::Intent;
use crate::rail_move::RailMove;
use crate::test_support::{inspect, open, run};

#[gpui::test]
fn selecting_and_moving_change_the_active_item(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let active = |harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
        inspect(harness, cx, |modal| {
            modal.state.page().map(|page| page.id.to_string())
        })
    };
    run(&harness, Intent::Select("terminal".into()), cx);
    assert_eq!(active(&harness, cx), Some("terminal".to_owned()));
    run(&harness, Intent::Move(RailMove::Next), cx);
    assert_eq!(active(&harness, cx), Some("browser".to_owned()));
}
