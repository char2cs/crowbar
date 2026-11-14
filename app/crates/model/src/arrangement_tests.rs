use super::*;
use crate::split_node::SplitNode;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

#[test]
fn pane_ids_lists_the_leaves_of_every_tab() {
    let shape = |tab: u64, pane: &str| TabShape {
        id: TabId(tab),
        title: String::new(),
        tree: Some(SplitNode::leaf(id(pane))),
        focused: Some(id(pane)),
    };
    let empty = TabShape {
        id: TabId(3),
        title: String::new(),
        tree: None,
        focused: None,
    };
    let arrangement = Arrangement {
        tabs: vec![shape(1, "a"), empty, shape(2, "b")],
        active: Some(TabId(1)),
    };
    assert_eq!(
        arrangement.pane_ids(),
        [id("a"), id("b")],
        "leaves in tab order"
    );
}
