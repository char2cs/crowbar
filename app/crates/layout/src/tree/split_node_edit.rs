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
    use crate::test_support::split_fixtures::{nested_left, panes, tiled, two_by_one};
    use crate::test_support::tab_fixtures::id;

    #[test]
    fn split_leaf_orders_children_by_edge_and_misses_an_unknown_target() {
        let cases = [
            (Edge::Left, ["n", "a"], Axis::Horizontal),
            (Edge::Right, ["a", "n"], Axis::Horizontal),
            (Edge::Top, ["n", "a"], Axis::Vertical),
            (Edge::Bottom, ["a", "n"], Axis::Vertical),
        ];
        for (edge, order, expected) in cases {
            let mut tree = SplitNode::leaf(id("a"));
            assert!(tree.split_leaf(&id("a"), SplitNode::leaf(id("n")), edge));
            assert_eq!(panes(&tree), order, "{edge:?} order");
            let SplitNode::Split { axis, ratio, .. } = tree else {
                panic!("{edge:?}: a split");
            };
            assert_eq!((axis, ratio), (expected, EQUAL_SHARE), "{edge:?}");
        }
        let mut tree = two_by_one();
        let missing = tree.split_leaf(&id("zz"), SplitNode::leaf(id("n")), Edge::Left);
        assert!(!missing, "missing target");
        assert_eq!(tree, two_by_one(), "unchanged");
    }

    #[test]
    fn without_collapses_the_parent_split_only() {
        let rest = two_by_one().without(&id("a"));
        assert_eq!(rest, Some(SplitNode::leaf(id("b"))), "sibling takes it");
        assert_eq!(SplitNode::leaf(id("a")).without(&id("a")), None, "last");
        assert_eq!(two_by_one().without(&id("zz")), Some(two_by_one()), "zz");
        let SplitNode::Split { first, second, .. } = tiled().without(&id("a")).unwrap() else {
            panic!("a split remains");
        };
        assert_eq!((*first, *second), (SplitNode::leaf(id("b")), SplitNode::leaf(id("c"))));
    }

    #[test]
    fn the_neighbour_is_the_pane_facing_the_closed_one() {
        let left_stack = {
            let mut tree = two_by_one();
            assert!(tree.split_leaf(&id("a"), SplitNode::leaf(id("c")), Edge::Bottom));
            tree
        };
        let cases = [
            (nested_left(), "x", Some("b")),
            (nested_left(), "b", Some("a")),
            (nested_left(), "a", Some("b")),
            (SplitNode::leaf(id("a")), "a", None),
            (tiled(), "a", Some("b")),
            (left_stack, "b", Some("c")),
        ];
        for (tree, closed, heir) in cases {
            let found = tree.neighbour_of(&id(closed)).map(PaneId::as_str);
            assert_eq!(found, heir, "{}: {closed}", panes(&tree).join(" "));
        }
    }

    #[test]
    fn split_at_mut_follows_paths_to_splits_only() {
        let mut tree = tiled();
        let cases: [(&[Branch], bool); 4] = [
            (&[], true),
            (&[Branch::Second], true),
            (&[Branch::First], false),
            (&[Branch::First, Branch::First], false),
        ];
        for (path, found) in cases {
            assert_eq!(tree.split_at_mut(path).is_some(), found, "{path:?}");
        }
    }
}
