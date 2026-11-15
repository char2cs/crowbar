use serde::{Deserialize, Serialize};

use crate::axis::Axis;

/// The side of a target pane a dropped or opened pane lands on.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Edge {
    /// Left of the target.
    Left,
    /// Right of the target.
    Right,
    /// Above the target.
    Top,
    /// Below the target.
    Bottom,
}

impl Edge {
    /// The axis of the split this edge creates.
    #[must_use]
    pub fn axis(self) -> Axis {
        match self {
            Self::Left | Self::Right => Axis::Horizontal,
            Self::Top | Self::Bottom => Axis::Vertical,
        }
    }

    /// Whether the new pane becomes the first child of the split.
    #[must_use]
    pub fn new_pane_first(self) -> bool {
        matches!(self, Self::Left | Self::Top)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn edges_map_to_axis_and_order() {
        let cases = [
            (Edge::Left, Axis::Horizontal, true),
            (Edge::Right, Axis::Horizontal, false),
            (Edge::Top, Axis::Vertical, true),
            (Edge::Bottom, Axis::Vertical, false),
        ];
        for (edge, axis, first) in cases {
            assert_eq!(edge.axis(), axis, "{edge:?} axis");
            assert_eq!(edge.new_pane_first(), first, "{edge:?} order");
        }
    }
}
