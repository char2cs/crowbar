use super::*;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

fn two_by_one() -> SplitNode {
    let mut tree = SplitNode::leaf(id("a"));
    assert!(tree.split_leaf(&id("a"), &id("b"), Edge::Right), "split a");
    tree
}

fn panes(tree: &SplitNode) -> Vec<String> {
    let mut out = Vec::new();
    tree.collect_panes(&mut out);
    out.iter().map(ToString::to_string).collect()
}

const BOUNDS: Rect = Rect {
    x: 0.0,
    y: 0.0,
    width: 100.0,
    height: 60.0,
};

#[test]
fn split_leaf_orders_children_by_edge() {
    let cases = [
        (Edge::Left, ["n", "a"], Axis::Horizontal),
        (Edge::Right, ["a", "n"], Axis::Horizontal),
        (Edge::Top, ["n", "a"], Axis::Vertical),
        (Edge::Bottom, ["a", "n"], Axis::Vertical),
    ];
    for (edge, order, expected_axis) in cases {
        let mut tree = SplitNode::leaf(id("a"));
        assert!(tree.split_leaf(&id("a"), &id("n"), edge), "{edge:?}");
        assert_eq!(panes(&tree), order, "{edge:?} order");
        assert!(
            matches!(tree, SplitNode::Split { axis, ratio, .. } if axis == expected_axis && (ratio - 0.5).abs() < f32::EPSILON),
            "{edge:?} shape"
        );
    }
}

#[test]
fn split_leaf_reports_a_missing_target_without_changing_the_tree() {
    let mut tree = two_by_one();
    let before = tree.clone();
    assert!(
        !tree.split_leaf(&id("zz"), &id("n"), Edge::Left),
        "missing target"
    );
    assert_eq!(tree, before, "unchanged");
}

#[test]
fn without_collapses_the_parent_split() {
    let tree = two_by_one();
    let rest = tree.clone().without(&id("a")).unwrap();
    assert_eq!(rest, SplitNode::leaf(id("b")), "sibling replaces the split");
    assert_eq!(
        SplitNode::leaf(id("a")).without(&id("a")),
        None,
        "last pane"
    );
    assert_eq!(tree.clone().without(&id("zz")), Some(tree), "unknown pane");
}

#[test]
fn without_keeps_ratios_of_untouched_splits() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
    let SplitNode::Split { first, second, .. } = tree.without(&id("a")).unwrap() else {
        panic!("a split remains");
    };
    assert_eq!(*first, SplitNode::leaf(id("b")), "b kept");
    assert_eq!(*second, SplitNode::leaf(id("c")), "c kept");
}

#[test]
fn sibling_is_the_first_pane_of_the_neighbouring_subtree() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
    assert_eq!(tree.sibling_of(&id("a")), Some(&id("b")), "a to b");
    assert_eq!(tree.sibling_of(&id("c")), Some(&id("b")), "c to b");
    assert_eq!(tree.sibling_of(&id("b")), Some(&id("c")), "b to c");
    assert_eq!(SplitNode::leaf(id("a")).sibling_of(&id("a")), None, "root");
}

#[test]
fn swap_leaves_exchanges_positions() {
    let mut tree = two_by_one();
    tree.swap_leaves(&id("a"), &id("b"));
    assert_eq!(panes(&tree), ["b", "a"], "swapped");
}

#[test]
fn place_divides_space_by_axis_and_ratio() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
    let mut slots = Vec::new();
    tree.place(BOUNDS, &mut slots);
    let rects: Vec<(String, [f32; 4])> = slots
        .iter()
        .map(|s| {
            (
                s.pane.to_string(),
                [s.rect.x, s.rect.y, s.rect.width, s.rect.height],
            )
        })
        .collect();
    assert_eq!(
        rects,
        [
            ("a".into(), [0.0, 0.0, 50.0, 60.0]),
            ("b".into(), [50.0, 0.0, 50.0, 30.0]),
            ("c".into(), [50.0, 30.0, 50.0, 30.0]),
        ],
        "tiles"
    );
}

#[test]
fn split_at_mut_follows_paths_to_splits_only() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
    assert!(tree.split_at_mut(&[]).is_some(), "root split");
    assert!(
        tree.split_at_mut(&[Branch::Second]).is_some(),
        "inner split"
    );
    assert!(tree.split_at_mut(&[Branch::First]).is_none(), "leaf");
    assert!(
        tree.split_at_mut(&[Branch::First, Branch::First]).is_none(),
        "past a leaf"
    );
}

#[test]
fn ratios_must_stay_in_range() {
    let mut tree = two_by_one();
    assert!(tree.ratios_valid(), "default");
    if let Some(SplitNode::Split { ratio, .. }) = tree.split_at_mut(&[]) {
        *ratio = 0.05;
    }
    assert!(!tree.ratios_valid(), "too small");
    if let Some(SplitNode::Split { ratio, .. }) = tree.split_at_mut(&[]) {
        *ratio = f32::NAN;
    }
    assert!(!tree.ratios_valid(), "nan");
}
