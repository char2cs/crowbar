use gpui::TestAppContext;

use crate::catalog::default_schema;
use crate::intent::Intent;
use crate::test_support::{inspect, open, run};

#[gpui::test]
fn every_control_kind_of_the_catalog_renders_without_a_notice(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let ids: Vec<_> = default_schema()
        .unwrap()
        .pages()
        .iter()
        .map(|page| page.id.clone())
        .collect();
    for id in ids {
        run(&harness, Intent::Select(id), cx);
    }
    assert!(inspect(&harness, cx, |modal| modal.notice.is_none()));
}
