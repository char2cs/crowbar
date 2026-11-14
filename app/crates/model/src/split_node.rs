use protocol::PaneId;
use serde::{Deserialize, Serialize};

use crate::axis::Axis;
use crate::branch::Branch;
use crate::edge::Edge;
use crate::pane_slot::PaneSlot;
use crate::rect::Rect;

/// Smallest share a split gives either child.
pub const MIN_RATIO: f32 = 0.1;

/// A node of the split tree: a pane, or a split with exactly two non-empty children.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "node", rename_all = "camelCase")]
pub enum SplitNode {
    /// A pane.
    Leaf {
        /// The pane shown here.
        pane: PaneId,
    },
    /// Two children sharing the available space.
    Split {
        /// How the children are arranged.
        axis: Axis,
        /// Share of the space given to the first child, within [`MIN_RATIO`]..`1 - MIN_RATIO`.
        ratio: f32,
        /// Left or top child.
        first: Box<SplitNode>,
        /// Right or bottom child.
        second: Box<SplitNode>,
    },
}

impl SplitNode {
    pub(crate) fn leaf(pane: PaneId) -> Self {
        Self::Leaf { pane }
    }

    pub(crate) fn collect_panes(&self, out: &mut Vec<PaneId>) {
        match self {
            Self::Leaf { pane } => out.push(pane.clone()),
            Self::Split { first, second, .. } => {
                first.collect_panes(out);
                second.collect_panes(out);
            },
        }
    }

    pub(crate) fn first_pane(&self) -> &PaneId {
        match self {
            Self::Leaf { pane } => pane,
            Self::Split { first, .. } => first.first_pane(),
        }
    }

    pub(crate) fn split_leaf(&mut self, target: &PaneId, new: &PaneId, edge: Edge) -> bool {
        match self {
            Self::Leaf { pane } if pane == target => {
                *self = Self::beside(pane.clone(), new.clone(), edge);
                true
            },
            Self::Leaf { .. } => false,
            Self::Split { first, second, .. } => {
                first.split_leaf(target, new, edge) || second.split_leaf(target, new, edge)
            },
        }
    }

    fn beside(existing: PaneId, new: PaneId, edge: Edge) -> Self {
        let (existing, fresh) = (Self::leaf(existing), Self::leaf(new));
        let (first, second) = if edge.new_pane_first() {
            (fresh, existing)
        } else {
            (existing, fresh)
        };
        Self::Split {
            axis: edge.axis(),
            ratio: 0.5,
            first: Box::new(first),
            second: Box::new(second),
        }
    }

    pub(crate) fn without(self, pane: &PaneId) -> Option<Self> {
        match self {
            Self::Leaf { pane: own } if &own == pane => None,
            Self::Leaf { .. } => Some(self),
            Self::Split {
                axis,
                ratio,
                first,
                second,
            } => match (first.without(pane), second.without(pane)) {
                (Some(first), Some(second)) => Some(Self::Split {
                    axis,
                    ratio,
                    first: Box::new(first),
                    second: Box::new(second),
                }),
                (kept, None) | (None, kept) => kept,
            },
        }
    }

    pub(crate) fn sibling_of(&self, pane: &PaneId) -> Option<&PaneId> {
        let Self::Split { first, second, .. } = self else {
            return None;
        };
        match (first.as_ref(), second.as_ref()) {
            (Self::Leaf { pane: own }, other) if own == pane => Some(other.first_pane()),
            (other, Self::Leaf { pane: own }) if own == pane => Some(other.first_pane()),
            _ => first.sibling_of(pane).or_else(|| second.sibling_of(pane)),
        }
    }

    pub(crate) fn swap_leaves(&mut self, a: &PaneId, b: &PaneId) {
        match self {
            Self::Leaf { pane } if pane == a => *pane = b.clone(),
            Self::Leaf { pane } if pane == b => *pane = a.clone(),
            Self::Leaf { .. } => {},
            Self::Split { first, second, .. } => {
                first.swap_leaves(a, b);
                second.swap_leaves(a, b);
            },
        }
    }

    pub(crate) fn split_at_mut(&mut self, path: &[Branch]) -> Option<&mut Self> {
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

    pub(crate) fn place(&self, bounds: Rect, out: &mut Vec<PaneSlot>) {
        match self {
            Self::Leaf { pane } => out.push(PaneSlot {
                pane: pane.clone(),
                rect: bounds,
            }),
            Self::Split {
                axis,
                ratio,
                first,
                second,
            } => {
                let (a, b) = divide(bounds, *axis, *ratio);
                first.place(a, out);
                second.place(b, out);
            },
        }
    }

    pub(crate) fn ratios_valid(&self) -> bool {
        match self {
            Self::Leaf { .. } => true,
            Self::Split {
                ratio,
                first,
                second,
                ..
            } => {
                ratio.is_finite()
                    && (MIN_RATIO..=1.0 - MIN_RATIO).contains(ratio)
                    && first.ratios_valid()
                    && second.ratios_valid()
            },
        }
    }
}

fn divide(bounds: Rect, axis: Axis, ratio: f32) -> (Rect, Rect) {
    match axis {
        Axis::Horizontal => {
            let width = bounds.width * ratio;
            (
                Rect { width, ..bounds },
                Rect {
                    x: bounds.x + width,
                    width: bounds.width - width,
                    ..bounds
                },
            )
        },
        Axis::Vertical => {
            let height = bounds.height * ratio;
            (
                Rect { height, ..bounds },
                Rect {
                    y: bounds.y + height,
                    height: bounds.height - height,
                    ..bounds
                },
            )
        },
    }
}

#[cfg(test)]
#[path = "split_node_tests.rs"]
mod tests;
