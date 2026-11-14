use gpui::TestAppContext;

use crate::intent::Intent;
use crate::test_support::{inspect, open, row, run};
use ui::TextInputEvent;

#[gpui::test]
fn a_search_hides_the_rows_that_do_not_match_and_keeps_those_that_do(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    run(
        &harness,
        Intent::Search(TextInputEvent::Changed("font size".to_owned())),
        cx,
    );
    let shown =
        |key: &str, harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
            let target = row(key);
            inspect(harness, cx, |modal| modal.state.row_visible(&target))
        };
    assert!(shown("uiFontSize", &harness, cx));
    assert!(!shown("themeMode", &harness, cx));
}
