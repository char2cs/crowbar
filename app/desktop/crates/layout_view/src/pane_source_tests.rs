use std::rc::Rc;

use super::PaneSource;
use crate::test_support::{FakeSource, pane_id};

#[test]
fn every_detached_pane_gets_a_tab_id_nobody_uses() {
    let source: Rc<dyn PaneSource> = Rc::new(FakeSource::new());
    let first = source.fresh_tab(&pane_id("a"));
    let second = source.fresh_tab(&pane_id("a"));
    assert_ne!(first.0, second.0, "two detaches never share a tab id");
}

#[test]
fn the_label_is_what_the_source_says_the_pane_is_called() {
    let source: Rc<dyn PaneSource> = Rc::new(FakeSource::new());
    assert_eq!(
        source.label(&pane_id("a")).as_ref(),
        "a",
        "label comes from the source"
    );
}
