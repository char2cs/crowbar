use gpui::TestAppContext;
use model::TabId;

use crate::test_support::{open, view};

#[gpui::test]
fn set_tabs_shows_the_tabs_and_the_active_one(cx: &mut TestAppContext) {
    let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 2);
    let (bar, cx) = (harness.bar, harness.cx);
    bar.read_with(cx, |bar, _| {
        assert_eq!(bar.tabs.len(), 2, "two tabs");
        assert_eq!(bar.active, Some(TabId(2)), "second is active");
    });
}

#[gpui::test]
fn tabs_created_by_a_drop_get_ids_counting_up_from_the_first_free_one(cx: &mut TestAppContext) {
    let harness = open(cx, vec![view(1, &["a"])], 1);
    let (bar, cx) = (harness.bar, harness.cx);
    let ids = bar.update(cx, |bar, _| {
        let first = bar.fresh_tab("One".into());
        let second = bar.fresh_tab("Two".into());
        (first.id, second.id)
    });
    assert_eq!(ids, (TabId(50), TabId(51)), "50 then 51");
}

#[test]
fn position_finds_a_tab_by_id() {
    let tabs = vec![view(4, &["a"]), view(9, &["b"])];
    assert_eq!(position(&tabs, TabId(9)), Some(1), "second slot");
    assert_eq!(position(&tabs, TabId(1)), None, "unknown tab");
}

use super::position;
