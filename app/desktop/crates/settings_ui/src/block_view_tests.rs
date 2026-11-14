use gpui::TestAppContext;

use crate::intent::Intent;
use crate::test_support::{inspect, open, run};
use ui::TextInputEvent;

#[gpui::test]
fn a_list_block_follows_the_search(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "providers");
    let any_block = |harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
        inspect(harness, cx, |modal| {
            let count = modal.state.page().map_or(0, |page| page.blocks.len());
            (0..count)
                .filter(|&index| modal.state.block_visible(index))
                .count()
        })
    };
    let all = any_block(&harness, cx);
    run(
        &harness,
        Intent::Search(TextInputEvent::Changed("descriptor".to_owned())),
        cx,
    );
    assert!(any_block(&harness, cx) <= all);
}
