use protocol::PaneId;

use crate::ide::pane_group::PaneGroup;
use crate::tree::axis::Axis;
use crate::tree::branch::Branch;
use crate::tree::edge::Edge;
use crate::tree::split_node::{EQUAL_SHARE, SplitNode};

impl SplitNode {
    pub(crate) fn split_leaf(
        &mut self,
        target: &PaneId,
        fresh: Self,
        edge: Edge,
    ) -> bool {
        match self {
            Self::Leaf(group) if group.contains(target) => {
                *self = Self::beside(Self::Leaf(group.clone()), fresh, edge);
                true
            },
            Self::Leaf(_) => false,
            Self::Split { first, .. } if first.group_of(target).is_some() => {
                first.split_leaf(target, fresh, edge)
            },
            Self::Split { second, .. } => second.split_leaf(target, fresh, edge),
        }
    }

    fn beside(
        existing: Self,
        fresh: Self,
        edge: Edge,
    ) -> Self {
        if edge.new_pane_first() {
            return Self::split(edge.axis(), EQUAL_SHARE, fresh, existing);
        }
        Self::split(edge.axis(), EQUAL_SHARE, existing, fresh)
    }

    fn split(
        axis: Axis,
        ratio: f32,
        first: Self,
        second: Self,
    ) -> Self {
        Self::Split {
            axis,
            ratio,
            first: Box::new(first),
            second: Box::new(second),
        }
    }

    pub(crate) fn without(
        self,
        pane: &PaneId,
    ) -> Option<Self> {
        self.pruned(pane, |mut group, pane| {
            group.take(pane);
            (!group.is_empty()).then_some(group)
        })
    }

    pub(crate) fn without_group_of(
        self,
        pane: &PaneId,
    ) -> Option<Self> {
        self.pruned(pane, |_, _| None)
    }

    fn pruned(
        self,
        pane: &PaneId,
        cut: fn(PaneGroup, &PaneId) -> Option<PaneGroup>,
    ) -> Option<Self> {
        match self {
            Self::Leaf(group) if group.contains(pane) => cut(group, pane).map(Self::Leaf),
            Self::Leaf(_) => Some(self),
            Self::Split {
                axis,
                ratio,
                first,
                second,
            } => match (first.pruned(pane, cut), second.pruned(pane, cut)) {
                (Some(first), Some(second)) => Some(Self::split(axis, ratio, first, second)),
                (kept, None) | (None, kept) => kept,
            },
        }
    }

    pub(crate) fn neighbour_of(
        &self,
        pane: &PaneId,
    ) -> Option<&PaneId> {
        let Self::Split { first, second, .. } = self else {
            return None;
        };
        if first.is_leaf_of(pane) {
            return second.first_pane();
        }
        if second.is_leaf_of(pane) {
            return first.last_pane();
        }
        first
            .neighbour_of(pane)
            .or_else(|| second.neighbour_of(pane))
    }

    pub(crate) fn swap_leaves(
        &mut self,
        a: &PaneId,
        b: &PaneId,
    ) {
        match self {
            Self::Leaf(group) => group.exchange(a, b),
            Self::Split { first, second, .. } => {
                first.swap_leaves(a, b);
                second.swap_leaves(a, b);
            },
        }
    }

    pub(crate) fn swap_groups(
        &mut self,
        a: &PaneId,
        b: &PaneId,
    ) {
        let (Some(first), Some(second)) = (self.group_of(a).cloned(), self.group_of(b).cloned())
        else {
            return;
        };
        self.exchange_groups(&first, &second);
    }

    fn exchange_groups(
        &mut self,
        x: &PaneGroup,
        y: &PaneGroup,
    ) {
        match self {
            Self::Leaf(group) if group == x => *group = y.clone(),
            Self::Leaf(group) if group == y => *group = x.clone(),
            Self::Leaf(_) => {},
            Self::Split { first, second, .. } => {
                first.exchange_groups(x, y);
                second.exchange_groups(x, y);
            },
        }
    }

    pub(crate) fn split_at_mut(
        &mut self,
        path: &[Branch],
    ) -> Option<&mut Self> {
        let Some((step, rest)) = path.split_first() else {
            return matches!(self, Self::Split { .. }).then_some(self);
        };
        let Self::Split { first, second, .. } = self else {
            return None;
        };
        match step {
            Branch::First => first.split_at_mut(rest),
            Branch::Second => second.split_at_mut(rest),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::split_fixtures::{nested_left, panes, two_by_one};
    use crate::test_support::tab_fixtures::id;

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
            assert!(
                tree.split_leaf(&id("a"), SplitNode::leaf(id("n")), edge),
                "{edge:?}"
            );
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
            !tree.split_leaf(&id("zz"), SplitNode::leaf(id("n")), Edge::Left),
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
        assert!(
            tree.split_leaf(&id("b"), SplitNode::leaf(id("c")), Edge::Bottom),
            "nest"
        );
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
        assert!(
            tree.split_leaf(&id("b"), SplitNode::leaf(id("c")), Edge::Bottom),
            "stack"
        );
        assert_eq!(tree.neighbour_of(&id("a")), Some(&id("b")), "a to the top");
        let mut left_stack = two_by_one();
        assert!(
            left_stack.split_leaf(&id("a"), SplitNode::leaf(id("c")), Edge::Bottom),
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
    fn split_at_mut_follows_paths_to_splits_only() {
        let mut tree = two_by_one();
        assert!(
            tree.split_leaf(&id("b"), SplitNode::leaf(id("c")), Edge::Bottom),
            "nest"
        );
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
}
