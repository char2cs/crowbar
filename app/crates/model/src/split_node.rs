use protocol::PaneId;
use serde::{Deserialize, Serialize};

use crate::axis::Axis;
use crate::branch::Branch;
use crate::division::Division;
use crate::edge::Edge;
use crate::pane_slot::PaneSlot;
use crate::rect::Rect;
use crate::sash::Sash;

/// Thickness of the divider between two panes, in logical pixels. Panes share what is left, so
/// the stored ratio is exactly the rendered ratio.
pub const SASH_THICKNESS: f32 = 6.0;

/// Smallest size either child of a split may have, in logical pixels. One value serves the live
/// drag, the committed ratio and the layout of a resized window; there is no second, relative
/// minimum.
pub const MIN_PANE_EXTENT: f32 = 160.0;

pub(crate) const EQUAL_SHARE: f32 = 0.5;

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
        /// Share of the space left after the sash that the first child gets, strictly between
        /// 0 and 1. A window too small for [`MIN_PANE_EXTENT`] on both sides shows equal halves.
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

    fn first_pane(&self) -> &PaneId {
        match self {
            Self::Leaf { pane } => pane,
            Self::Split { first, .. } => first.first_pane(),
        }
    }

    fn last_pane(&self) -> &PaneId {
        match self {
            Self::Leaf { pane } => pane,
            Self::Split { second, .. } => second.last_pane(),
        }
    }

    fn is_leaf_of(&self, id: &PaneId) -> bool {
        matches!(self, Self::Leaf { pane } if pane == id)
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
            ratio: EQUAL_SHARE,
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

    pub(crate) fn neighbour_of(&self, pane: &PaneId) -> Option<&PaneId> {
        let Self::Split { first, second, .. } = self else {
            return None;
        };
        if first.is_leaf_of(pane) {
            return Some(second.first_pane());
        }
        if second.is_leaf_of(pane) {
            return Some(first.last_pane());
        }
        first
            .neighbour_of(pane)
            .or_else(|| second.neighbour_of(pane))
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

    pub(crate) fn split_rect(&self, path: &[Branch], bounds: Rect) -> Option<(Axis, Rect)> {
        let Self::Split {
            axis,
            ratio,
            first,
            second,
        } = self
        else {
            return None;
        };
        let Some((step, rest)) = path.split_first() else {
            return Some((*axis, bounds));
        };
        let parts = divide(bounds, *axis, *ratio);
        match step {
            Branch::First => first.split_rect(rest, parts.first),
            Branch::Second => second.split_rect(rest, parts.second),
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
                let parts = divide(bounds, *axis, *ratio);
                first.place(parts.first, out);
                second.place(parts.second, out);
            },
        }
    }

    pub(crate) fn place_sashes(&self, bounds: Rect, path: &mut Vec<Branch>, out: &mut Vec<Sash>) {
        let Self::Split {
            axis,
            ratio,
            first,
            second,
        } = self
        else {
            return;
        };
        let parts = divide(bounds, *axis, *ratio);
        out.push(Sash {
            path: path.clone(),
            axis: *axis,
            rect: parts.sash,
        });
        path.push(Branch::First);
        first.place_sashes(parts.first, path, out);
        path.pop();
        path.push(Branch::Second);
        second.place_sashes(parts.second, path, out);
        path.pop();
    }

    pub(crate) fn set_ratio(&mut self, value: f32) {
        if let Self::Split { ratio, .. } = self {
            *ratio = value;
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
                    && *ratio > 0.0
                    && *ratio < 1.0
                    && first.ratios_valid()
                    && second.ratios_valid()
            },
        }
    }
}

pub(crate) fn extent_of(bounds: Rect, axis: Axis) -> f32 {
    match axis {
        Axis::Horizontal => bounds.width,
        Axis::Vertical => bounds.height,
    }
}

fn shareable(total: f32) -> f32 {
    (total - SASH_THICKNESS).max(0.0)
}

fn honours_minimum(shareable: f32) -> bool {
    shareable >= 2.0 * MIN_PANE_EXTENT
}

fn first_extent(total: f32, ratio: f32) -> f32 {
    let room = shareable(total);
    if !honours_minimum(room) {
        return room * EQUAL_SHARE;
    }
    (room * ratio).clamp(MIN_PANE_EXTENT, room - MIN_PANE_EXTENT)
}

pub(crate) fn ratio_for(total: f32, wanted: f32) -> Option<f32> {
    let room = shareable(total);
    honours_minimum(room).then(|| wanted.clamp(MIN_PANE_EXTENT, room - MIN_PANE_EXTENT) / room)
}

fn divide(bounds: Rect, axis: Axis, ratio: f32) -> Division {
    let first = first_extent(extent_of(bounds, axis), ratio);
    let rest = (extent_of(bounds, axis) - first - SASH_THICKNESS).max(0.0);
    match axis {
        Axis::Horizontal => Division {
            first: Rect {
                width: first,
                ..bounds
            },
            sash: Rect {
                x: bounds.x + first,
                width: SASH_THICKNESS,
                ..bounds
            },
            second: Rect {
                x: bounds.x + first + SASH_THICKNESS,
                width: rest,
                ..bounds
            },
        },
        Axis::Vertical => Division {
            first: Rect {
                height: first,
                ..bounds
            },
            sash: Rect {
                y: bounds.y + first,
                height: SASH_THICKNESS,
                ..bounds
            },
            second: Rect {
                y: bounds.y + first + SASH_THICKNESS,
                height: rest,
                ..bounds
            },
        },
    }
}

#[cfg(test)]
#[path = "split_node_tests.rs"]
mod tests;
