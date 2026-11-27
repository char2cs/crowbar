use protocol::PaneId;
use serde::{Deserialize, Serialize};

use crate::ide::pane_group::PaneGroup;
use crate::tree::axis::Axis;

pub const SASH_THICKNESS: f32 = 6.0;

pub const MIN_PANE_EXTENT: f32 = 160.0;

pub(crate) const EQUAL_SHARE: f32 = 0.5;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "node", rename_all = "camelCase")]
pub enum SplitNode {
    Leaf(PaneGroup),
    Split {
        axis: Axis,
        ratio: f32,
        first: Box<SplitNode>,
        second: Box<SplitNode>,
    },
}

impl SplitNode {
    pub(crate) fn leaf(pane: PaneId) -> Self {
        Self::Leaf(PaneGroup::single(pane))
    }

    pub(crate) fn pane_ids(&self) -> Vec<PaneId> {
        match self {
            Self::Leaf(group) => group.members(),
            Self::Split { first, second, .. } => [first.pane_ids(), second.pane_ids()].concat(),
        }
    }

    pub(crate) fn first_pane(&self) -> Option<&PaneId> {
        match self {
            Self::Leaf(group) => group.shown(),
            Self::Split { first, .. } => first.first_pane(),
        }
    }

    pub(crate) fn last_pane(&self) -> Option<&PaneId> {
        match self {
            Self::Leaf(group) => group.shown(),
            Self::Split { second, .. } => second.last_pane(),
        }
    }

    pub(crate) fn is_leaf_of(
        &self,
        id: &PaneId,
    ) -> bool {
        matches!(self, Self::Leaf(group) if group.contains(id))
    }

    pub(crate) fn group_of(
        &self,
        pane: &PaneId,
    ) -> Option<&PaneGroup> {
        match self {
            Self::Leaf(group) => group.contains(pane).then_some(group),
            Self::Split { first, second, .. } => {
                first.group_of(pane).or_else(|| second.group_of(pane))
            },
        }
    }

    pub(crate) fn group_of_mut(
        &mut self,
        pane: &PaneId,
    ) -> Option<&mut PaneGroup> {
        match self {
            Self::Leaf(group) => group.contains(pane).then_some(group),
            Self::Split { first, second, .. } => match first.group_of_mut(pane) {
                Some(found) => Some(found),
                None => second.group_of_mut(pane),
            },
        }
    }

    pub(crate) fn clear_previews(&mut self) {
        match self {
            Self::Leaf(group) => group.preview = None,
            Self::Split { first, second, .. } => {
                first.clear_previews();
                second.clear_previews();
            },
        }
    }

    pub(crate) fn leaf_count(&self) -> usize {
        match self {
            Self::Leaf(_) => 1,
            Self::Split { first, second, .. } => first.leaf_count() + second.leaf_count(),
        }
    }

    pub(crate) fn set_ratio(
        &mut self,
        value: f32,
    ) {
        if let Self::Split { ratio, .. } = self {
            *ratio = value;
        }
    }

    pub(crate) fn ratios_valid(&self) -> bool {
        match self {
            Self::Leaf(_) => true,
            Self::Split {
                ratio,
                first,
                second,
                ..
            } => {
                ratio.is_finite()
                    && *ratio > 0.0
                    && *ratio < 1.0
                    && first.ratios_valid()
                    && second.ratios_valid()
            },
        }
    }

    pub(crate) fn groups_valid(&self) -> bool {
        match self {
            Self::Leaf(group) => group.is_consistent(),
            Self::Split { first, second, .. } => first.groups_valid() && second.groups_valid(),
        }
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::split_fixtures::two_by_one;

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
}
