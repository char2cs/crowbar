use crate::edge::Edge;
use crate::rect::Rect;
use crate::split_node::SplitNode;
use crate::test_support::tab_fixtures::id;

pub(crate) fn two_by_one() -> SplitNode {
    let mut tree = SplitNode::leaf(id("a"));
    assert!(
        tree.split_leaf(&id("a"), SplitNode::leaf(id("b")), Edge::Right),
        "split a"
    );
    tree
}

/// `[[a|b]|x]`: the layout whose right pane used to hand focus to the far left.
pub(crate) fn nested_left() -> SplitNode {
    let mut tree = SplitNode::leaf(id("a"));
    assert!(
        tree.split_leaf(&id("a"), SplitNode::leaf(id("x")), Edge::Right),
        "x right of a"
    );
    assert!(
        tree.split_leaf(&id("a"), SplitNode::leaf(id("b")), Edge::Right),
        "b right of a"
    );
    tree
}

pub(crate) fn panes(tree: &SplitNode) -> Vec<String> {
    let out = tree.pane_ids();
    out.iter().map(ToString::to_string).collect()
}

pub(crate) fn bounds(
    width: f32,
    height: f32,
) -> Rect {
    Rect {
        x: 0.0,
        y: 0.0,
        width,
        height,
    }
}

pub(crate) fn rects(
    tree: &SplitNode,
    bounds: Rect,
) -> Vec<(String, [f32; 4])> {
    let slots = tree.place_panes(bounds);
    slots
        .iter()
        .map(|s| {
            (
                s.pane.to_string(),
                [s.rect.x, s.rect.y, s.rect.width, s.rect.height],
            )
        })
        .collect()
}
