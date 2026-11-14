use gpui::TestAppContext;

use crate::intent::Intent;
use crate::test_support::{inspect, open, run};
use ui::TextInputEvent;

#[gpui::test]
fn a_search_keeps_only_the_matching_shortcuts(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "keybindings");
    let count = |harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
        inspect(harness, cx, |modal| {
            modal
                .state
                .schema()
                .shortcuts()
                .filter(|(_, shortcut)| modal.state.shortcut_visible(shortcut))
                .count()
        })
    };
    let all = count(&harness, cx);
    run(
        &harness,
        Intent::Search(TextInputEvent::Changed("console".to_owned())),
        cx,
    );
    assert_eq!((all, count(&harness, cx)), (24, 1));
}
