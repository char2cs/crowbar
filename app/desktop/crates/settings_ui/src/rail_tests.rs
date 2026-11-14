use gpui::TestAppContext;

use crate::intent::Intent;
use crate::test_support::{inspect, open, run};
use ui::TextInputEvent;

#[gpui::test]
fn a_search_narrows_the_rail_and_clearing_it_brings_every_page_back(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let pages = |harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
        inspect(harness, cx, |modal| modal.state.visible_pages().len())
    };
    run(
        &harness,
        Intent::Search(TextInputEvent::Changed("scrollback".to_owned())),
        cx,
    );
    assert_eq!(pages(&harness, cx), 1);
    run(&harness, Intent::Search(TextInputEvent::Cancelled), cx);
    assert_eq!(pages(&harness, cx), 10);
}
