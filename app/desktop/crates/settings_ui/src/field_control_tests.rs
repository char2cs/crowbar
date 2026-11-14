use gpui::TestAppContext;

use crate::intent::Intent;
use crate::test_support::{inspect, open, row, run};
use ui::TextInputEvent;

#[gpui::test]
fn an_invalid_search_template_is_kept_but_flagged(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "browser");
    let target = row("browserCustomSearchUrl");
    let typed = TextInputEvent::Changed("not a url".to_owned());
    run(&harness, Intent::Edit(target.clone(), typed), cx);
    run(&harness, Intent::Commit(target.clone()), cx);
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .issue(&target)
        .is_some()));
}
