use crate::axis::Axis;
use crate::branch::Branch;
use crate::division::Division;
use crate::group_slot::GroupSlot;
use crate::pane_slot::PaneSlot;
use crate::rect::Rect;
use crate::sash::Sash;
use crate::split_extent::{extent_of, first_extent};
use crate::split_node::{SASH_THICKNESS, SplitNode};

impl SplitNode {
    pub(crate) fn split_rect(
        &self,
        path: &[Branch],
        bounds: Rect,
    ) -> Option<(Axis, Rect)> {
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

    pub(crate) fn place_panes(
        &self,
        bounds: Rect,
    ) -> Vec<PaneSlot> {
        match self {
            Self::Leaf(group) => vec![PaneSlot {
                pane: group.active().clone(),
                rect: bounds,
            }],
            Self::Split {
                axis,
                ratio,
                first,
                second,
            } => {
                let parts = divide(bounds, *axis, *ratio);
                [
                    first.place_panes(parts.first),
                    second.place_panes(parts.second),
                ]
                .concat()
            },
        }
    }

    pub(crate) fn place_groups(
        &self,
        bounds: Rect,
    ) -> Vec<GroupSlot> {
        match self {
            Self::Leaf(group) => vec![GroupSlot {
                rect: bounds,
                panes: group.panes().to_vec(),
                active: group.active().clone(),
            }],
            Self::Split {
                axis,
                ratio,
                first,
                second,
            } => {
                let parts = divide(bounds, *axis, *ratio);
                [
                    first.place_groups(parts.first),
                    second.place_groups(parts.second),
                ]
                .concat()
            },
        }
    }

    pub(crate) fn place_sashes(
        &self,
        bounds: Rect,
        path: &[Branch],
    ) -> Vec<Sash> {
        let Self::Split {
            axis,
            ratio,
            first,
            second,
        } = self
        else {
            return Vec::new();
        };
        let parts = divide(bounds, *axis, *ratio);
        let here = Sash {
            path: path.to_vec(),
            axis: *axis,
            rect: parts.sash,
        };
        [
            vec![here],
            first.place_sashes(parts.first, &[path, &[Branch::First]].concat()),
            second.place_sashes(parts.second, &[path, &[Branch::Second]].concat()),
        ]
        .concat()
    }
}

fn divide(
    bounds: Rect,
    axis: Axis,
    ratio: f32,
) -> Division {
    let first = first_extent(extent_of(bounds, axis), ratio);
    let rest = (extent_of(bounds, axis) - first - SASH_THICKNESS).max(0.0);
    match axis {
        Axis::Horizontal => divide_across(bounds, first, rest),
        Axis::Vertical => divide_down(bounds, first, rest),
    }
}

fn divide_across(
    bounds: Rect,
    first: f32,
    rest: f32,
) -> Division {
    let before = Rect {
        width: first,
        ..bounds
    };
    let sash = Rect {
        x: bounds.x + first,
        width: SASH_THICKNESS,
        ..bounds
    };
    let after = Rect {
        x: bounds.x + first + SASH_THICKNESS,
        width: rest,
        ..bounds
    };
    Division {
        first: before,
        sash,
        second: after,
    }
}

fn divide_down(
    bounds: Rect,
    first: f32,
    rest: f32,
) -> Division {
    let before = Rect {
        height: first,
        ..bounds
    };
    let sash = Rect {
        y: bounds.y + first,
        height: SASH_THICKNESS,
        ..bounds
    };
    let after = Rect {
        y: bounds.y + first + SASH_THICKNESS,
        height: rest,
        ..bounds
    };
    Division {
        first: before,
        sash,
        second: after,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::edge::Edge;
    use crate::split_node::MIN_PANE_EXTENT;
    use crate::test_support::split_fixtures::{bounds, rects, two_by_one};
    use crate::test_support::tab_fixtures::id;

    type SashFacts = (Vec<Branch>, Axis, [f32; 4]);

    #[test]
    fn place_leaves_room_for_the_sash_and_divides_nested_splits_by_axis() {
        let mut nested = two_by_one();
        assert!(nested.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
        let cases = [
            (
                "500 + 6 sash + 500",
                two_by_one(),
                bounds(1006.0, 600.0),
                vec![
                    ("a".to_string(), [0.0, 0.0, 500.0, 600.0]),
                    ("b".to_string(), [506.0, 0.0, 500.0, 600.0]),
                ],
            ),
            (
                "tiles",
                nested,
                bounds(1006.0, 1006.0),
                vec![
                    ("a".to_string(), [0.0, 0.0, 500.0, 1006.0]),
                    ("b".to_string(), [506.0, 0.0, 500.0, 500.0]),
                    ("c".to_string(), [506.0, 506.0, 500.0, 500.0]),
                ],
            ),
        ];
        for (name, tree, area, expected) in cases {
            assert_eq!(rects(&tree, area), expected, "{name}");
        }
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
    fn sashes_are_listed_with_their_path_outermost_first() {
        let mut tree = two_by_one();
        assert!(tree.split_leaf(&id("b"), &id("c"), Edge::Bottom), "nest");
        let sashes = tree.place_sashes(bounds(1006.0, 1006.0), &[]);
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
}
