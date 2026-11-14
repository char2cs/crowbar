use gpui::TestAppContext;

use crate::intent::Intent;
use crate::test_support::{inspect, open, run};
use ui::TextInputEvent;

#[gpui::test]
fn a_search_with_no_match_leaves_no_page_to_show(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    run(
        &harness,
        Intent::Search(TextInputEvent::Changed("zzzz".to_owned())),
        cx,
    );
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .visible_pages()
        .is_empty()));
}
