use crate::ide::pane_group::PaneGroup;
use crate::tree::axis::Axis;
use crate::tree::branch::Branch;
use crate::tree::geometry::division::Division;
use crate::tree::geometry::pane_slot::PaneSlot;
use crate::tree::geometry::rect::Rect;
use crate::tree::geometry::sash::Sash;
use crate::tree::split_extent::{extent_of, first_extent};
use crate::tree::split_node::{SASH_THICKNESS, SplitNode};

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
            Self::Leaf(group) => group
                .shown()
                .map(|pane| PaneSlot {
                    pane: pane.clone(),
                    rect: bounds,
                })
                .into_iter()
                .collect(),
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

    pub(crate) fn place_leaves(
        &self,
        bounds: Rect,
    ) -> Vec<(Rect, &PaneGroup)> {
        match self {
            Self::Leaf(group) => vec![(bounds, group)],
            Self::Split {
                axis,
                ratio,
                first,
                second,
            } => {
                let parts = divide(bounds, *axis, *ratio);
                [
                    first.place_leaves(parts.first),
                    second.place_leaves(parts.second),
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

pub(crate) fn divide(
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
    use crate::test_support::split_fixtures::{bounds, rects, tiled, two_by_one};
    use crate::tree::split_node::MIN_PANE_EXTENT;

    type SashFacts = (Vec<Branch>, Axis, [f32; 4]);

    fn widths(
        tree: &SplitNode,
        width: f32,
    ) -> Vec<f32> {
        let slots = rects(tree, bounds(width, 100.0));
        slots.iter().map(|slot| slot.1[2]).collect()
    }

    #[test]
    fn place_leaves_room_for_the_sash_and_divides_nested_splits_by_axis() {
        let side_by_side = rects(&two_by_one(), bounds(1006.0, 600.0));
        let expected = [[0.0, 0.0, 500.0, 600.0], [506.0, 0.0, 500.0, 600.0]];
        let named = [("a".to_owned(), expected[0]), ("b".to_owned(), expected[1])];
        assert_eq!(side_by_side, named, "500 + 6 sash + 500");
        let tiles: Vec<[f32; 4]> = rects(&tiled(), bounds(1006.0, 1006.0))
            .into_iter()
            .map(|slot| slot.1)
            .collect();
        let expected = [
            [0.0, 0.0, 500.0, 1006.0],
            [506.0, 0.0, 500.0, 500.0],
            [506.0, 506.0, 500.0, 500.0],
        ];
        assert_eq!(tiles, expected, "tiles");
    }

    #[test]
    fn the_pixel_minimum_holds_unless_the_window_is_too_small_for_it() {
        let mut tree = two_by_one();
        tree.set_ratio(0.01);
        let held = [MIN_PANE_EXTENT, 1000.0 - MIN_PANE_EXTENT];
        assert_eq!(widths(&tree, 1006.0), held, "held at the minimum");
        tree.set_ratio(0.9);
        let small = widths(&tree, 2.0 * MIN_PANE_EXTENT + SASH_THICKNESS - 1.0);
        assert_eq!(small.first(), small.get(1), "equal halves: {small:?}");
        let tiny = widths(&tree, 1.0);
        assert!(tiny.iter().all(|width| *width >= 0.0), "never negative");
    }

    #[test]
    fn sashes_and_split_rects_follow_paths_outermost_first() {
        let tree = tiled();
        let sashes = tree.place_sashes(bounds(1006.0, 1006.0), &[]);
        let facts = |s: &crate::tree::geometry::sash::Sash| {
            (s.path.clone(), s.axis, [s.rect.x, s.rect.y, s.rect.width, s.rect.height])
        };
        let found: Vec<SashFacts> = sashes.iter().map(facts).collect();
        let outer = (vec![], Axis::Horizontal, [500.0, 0.0, 6.0, 1006.0]);
        let inner = (vec![Branch::Second], Axis::Vertical, [506.0, 500.0, 500.0, 6.0]);
        assert_eq!(found, [outer, inner], "sashes");
        let (axis, rect) = tree.split_rect(&[Branch::Second], bounds(1006.0, 1006.0)).unwrap();
        let owned = [rect.x, rect.y, rect.width, rect.height];
        assert_eq!((axis, owned), (Axis::Vertical, [506.0, 0.0, 500.0, 1006.0]));
        let leaf = tree.split_rect(&[Branch::First], bounds(1.0, 1.0));
        assert!(leaf.is_none(), "leaf");
    }
}
