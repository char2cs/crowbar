use super::*;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

fn two_by_one() -> SplitNode {
    let mut tree = SplitNode::leaf(id("a"));
    assert!(tree.split_leaf(&id("a"), &id("b"), Edge::Right), "split a");
    tree
}

/// `[[a|b]|x]`: the layout whose right pane used to hand focus to the far left.
fn nested_left() -> SplitNode {
    let mut tree = SplitNode::leaf(id("a"));
    assert!(
        tree.split_leaf(&id("a"), &id("x"), Edge::Right),
        "x right of a"
    );
    assert!(
        tree.split_leaf(&id("a"), &id("b"), Edge::Right),
        "b right of a"
    );
    tree
}

fn panes(tree: &SplitNode) -> Vec<String> {
    let mut out = Vec::new();
    tree.collect_panes(&mut out);
    out.iter().map(ToString::to_string).collect()
}

fn bounds(width: f32, height: f32) -> Rect {
    Rect {
        x: 0.0,
        y: 0.0,
        width,
        height,
    }
}

fn rects(tree: &SplitNode, bounds: Rect) -> Vec<(String, [f32; 4])> {
    let mut slots = Vec::new();
    tree.place(bounds, &mut slots);
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
            matches!(tree, SplitNode::Split { axis, ratio, .. } if axis == expected_axis && (ratio - EQUAL_SHARE).abs() < f32::EPSILON),
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
fn the_neighbour_is_the_pane_next_to_the_closed_one() {
    let tree = nested_left();
    assert_eq!(
        tree.neighbour_of(&id("x")),
        Some(&id("b")),
        "closing the right pane focuses the pane beside it, not the far left"
    );
    assert_eq!(tree.neighbour_of(&id("b")), Some(&id("a")), "b to a");
    assert_eq!(tree.neighbour_of(&id("a")), Some(&id("b")), "a to b");
    assert_eq!(
        SplitNode::leaf(id("a")).neighbour_of(&id("a")),
        None,
        "root"
    );
}

#[test]
fn the_neighbour_of_a_pane_beside_a_stack_is_the_stack_side_facing_it() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "stack");
    assert_eq!(tree.neighbour_of(&id("a")), Some(&id("b")), "a to the top");
    let mut left_stack = two_by_one();
    assert!(
        left_stack.split_leaf(&id("a"), &id("c"), Edge::Bottom),
        "stack"
    );
    assert_eq!(
        left_stack.neighbour_of(&id("b")),
        Some(&id("c")),
        "b to the bottom of the left stack, the last pane facing it"
    );
}

#[test]
fn swap_leaves_exchanges_positions() {
    let mut tree = two_by_one();
    tree.swap_leaves(&id("a"), &id("b"));
    assert_eq!(panes(&tree), ["b", "a"], "swapped");
}

#[test]
fn place_leaves_room_for_the_sash_and_the_ratio_applies_to_what_is_left() {
    let tree = two_by_one();
    assert_eq!(
        rects(&tree, bounds(1006.0, 600.0)),
        [
            ("a".into(), [0.0, 0.0, 500.0, 600.0]),
            ("b".into(), [506.0, 0.0, 500.0, 600.0]),
        ],
        "500 + 6 sash + 500"
    );
}

#[test]
fn place_divides_nested_splits_by_axis() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
    assert_eq!(
        rects(&tree, bounds(1006.0, 1006.0)),
        [
            ("a".into(), [0.0, 0.0, 500.0, 1006.0]),
            ("b".into(), [506.0, 0.0, 500.0, 500.0]),
            ("c".into(), [506.0, 506.0, 500.0, 500.0]),
        ],
        "tiles"
    );
}

#[test]
fn a_ratio_below_the_pixel_minimum_is_held_at_the_minimum() {
    let mut tree = two_by_one();
    tree.set_ratio(0.01);
    let slots = rects(&tree, bounds(1006.0, 600.0));
    assert_eq!(
        slots.first().map(|s| s.1[2]),
        Some(MIN_PANE_EXTENT),
        "first"
    );
    assert_eq!(
        slots.get(1).map(|s| s.1[2]),
        Some(1000.0 - MIN_PANE_EXTENT),
        "second takes the rest"
    );
}

#[test]
fn a_window_too_small_for_the_minimum_shows_equal_halves() {
    let mut tree = two_by_one();
    tree.set_ratio(0.9);
    let slots = rects(
        &tree,
        bounds(2.0 * MIN_PANE_EXTENT + SASH_THICKNESS - 1.0, 100.0),
    );
    let widths: Vec<f32> = slots.iter().map(|s| s.1[2]).collect();
    assert_eq!(widths.first(), widths.get(1), "equal: {widths:?}");
    let slots = rects(&tree, bounds(1.0, 100.0));
    assert!(slots.iter().all(|s| s.1[2] >= 0.0), "never negative");
}

#[test]
fn ratio_for_applies_the_same_minimum_as_the_layout() {
    let total = 1006.0;
    let room = 1000.0;
    let cases = [
        (0.0, MIN_PANE_EXTENT),
        (MIN_PANE_EXTENT - 1.0, MIN_PANE_EXTENT),
        (300.0, 300.0),
        (room, room - MIN_PANE_EXTENT),
    ];
    for (wanted, shown) in cases {
        let ratio = ratio_for(total, wanted).unwrap();
        let mut tree = two_by_one();
        tree.set_ratio(ratio);
        let first = rects(&tree, bounds(total, 100.0)).first().map(|s| s.1[2]);
        assert!(
            first.is_some_and(|w| (w - shown).abs() < 0.01),
            "wanted {wanted}: drag shows {shown}, layout gave {first:?}"
        );
    }
    assert_eq!(ratio_for(2.0 * MIN_PANE_EXTENT, 100.0), None, "too small");
}

type SashFacts = (Vec<Branch>, Axis, [f32; 4]);

#[test]
fn sashes_are_listed_with_their_path_outermost_first() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
    let mut sashes = Vec::new();
    tree.place_sashes(bounds(1006.0, 1006.0), &mut Vec::new(), &mut sashes);
    let found: Vec<SashFacts> = sashes
        .iter()
        .map(|s| {
            (
                s.path.clone(),
                s.axis,
                [s.rect.x, s.rect.y, s.rect.width, s.rect.height],
            )
        })
        .collect();
    assert_eq!(
        found,
        [
            (vec![], Axis::Horizontal, [500.0, 0.0, 6.0, 1006.0]),
            (
                vec![Branch::Second],
                Axis::Vertical,
                [506.0, 500.0, 500.0, 6.0]
            ),
        ],
        "sashes"
    );
}

#[test]
fn split_rect_follows_paths_with_the_real_sizes() {
    let mut tree = two_by_one();
    assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
    let (axis, rect) = tree
        .split_rect(&[Branch::Second], bounds(1006.0, 1006.0))
        .unwrap();
    assert_eq!(axis, Axis::Vertical, "inner axis");
    assert_eq!(
        [rect.x, rect.y, rect.width, rect.height],
        [506.0, 0.0, 500.0, 1006.0],
        "the space the inner split owns"
    );
    assert!(
        tree.split_rect(&[Branch::First], bounds(1.0, 1.0))
            .is_none(),
        "leaf"
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
fn ratios_must_be_strictly_between_zero_and_one() {
    let mut tree = two_by_one();
    assert!(tree.ratios_valid(), "default");
    for bad in [0.0, 1.0, -0.5, 2.0, f32::NAN, f32::INFINITY] {
        tree.set_ratio(bad);
        assert!(!tree.ratios_valid(), "{bad}");
    }
    tree.set_ratio(0.05);
    assert!(
        tree.ratios_valid(),
        "a small ratio is fine; the pixel minimum is applied on layout"
    );
}
