use gpui::TestAppContext;

use crate::intent::Intent;
use crate::key::Key;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{inspect, open, row, run};
use ui::TextInputEvent;

#[gpui::test]
fn a_cancelled_search_clears_the_query(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    run(
        &harness,
        Intent::Search(TextInputEvent::Changed("git".to_owned())),
        cx,
    );
    run(&harness, Intent::Search(TextInputEvent::Cancelled), cx);
    assert_eq!(
        inspect(&harness, cx, |modal| modal.state.query().to_owned()),
        ""
    );
}

#[gpui::test]
fn submitted_text_commits_and_cancelled_text_reverts(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "browser");
    let target = row("browserHomepage");
    run(
        &harness,
        Intent::Edit(
            target.clone(),
            TextInputEvent::Changed("https://a.test".to_owned()),
        ),
        cx,
    );
    run(
        &harness,
        Intent::Edit(target.clone(), TextInputEvent::Cancelled),
        cx,
    );
    assert_eq!(harness.backend.get(&Key::new("browserHomepage")), None);
    run(
        &harness,
        Intent::Edit(
            target.clone(),
            TextInputEvent::Changed("https://b.test".to_owned()),
        ),
        cx,
    );
    run(
        &harness,
        Intent::Edit(target, TextInputEvent::Submitted),
        cx,
    );
    assert!(harness.backend.get(&Key::new("browserHomepage")).is_some());
}
