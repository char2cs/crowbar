use gpui::TestAppContext;

use super::*;
use crate::test_support::{inspect, open};

#[gpui::test]
fn deliver_and_send_both_reach_the_modal(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let owner = harness.modal.clone();
    cx.update(|window, cx| deliver(&owner, Intent::Select("git".into()), (window, cx)));
    assert_eq!(
        inspect(&harness, cx, |modal| modal
            .state
            .page()
            .map(|p| p.id.to_string())),
        Some("git".to_owned())
    );
    let handler = send::<()>(&owner, Intent::Select("editor".into()));
    cx.update(|window, cx| handler(&(), window, cx));
    assert_eq!(
        inspect(&harness, cx, |modal| modal
            .state
            .page()
            .map(|p| p.id.to_string())),
        Some("editor".to_owned())
    );
}
