use super::*;
use crate::test_support::prelude::*;

#[gpui::test]
fn every_visible_row_is_drawn(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    let (shown, listed) = sidebar.update_in(cx, |this, _, cx| {
        let project = this.data.active_project().expect("an active space");
        let tree = this.data.tree(&project);
        let handler = Sidebar::row_handler(cx);
        (this.tree_rows(&tree, &handler).len(), tree.rows.len())
    });
    assert!(shown > 0 && shown <= listed + 1);
}

#[gpui::test]
fn folding_a_parent_draws_fewer_rows(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    let count = |sidebar: &gpui::Entity<Sidebar>, cx: &mut gpui::VisualTestContext| {
        sidebar.update_in(cx, |this, _, cx| {
            let project = this.data.active_project().expect("an active space");
            let tree = this.data.tree(&project);
            this.tree_rows(&tree, &Sidebar::row_handler(cx)).len()
        })
    };
    let before = count(&sidebar, cx);
    sidebar.update(cx, |this, _| this.tree.toggle_fold(&RowId::from("demo")));
    assert!(count(&sidebar, cx) < before);
}
