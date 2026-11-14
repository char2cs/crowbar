use gpui::TestAppContext;

use crate::intent::Intent;
use crate::test_support::{inspect, open, row, run};

#[gpui::test]
fn an_invalid_row_swaps_its_description_for_the_invalid_text(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "browser");
    let target = row("browserCustomSearchUrl");
    let normal = inspect(&harness, cx, |modal| {
        modal.state.description(&target).map(str::to_owned)
    });
    let typed = ui::TextInputEvent::Changed("nope".to_owned());
    run(&harness, Intent::Edit(target.clone(), typed), cx);
    let invalid = inspect(&harness, cx, |modal| {
        modal.state.description(&target).map(str::to_owned)
    });
    assert_ne!(normal, invalid);
}
