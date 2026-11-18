use crate::axis::Axis;
use crate::rect::Rect;
use crate::split_node::{EQUAL_SHARE, MIN_PANE_EXTENT, SASH_THICKNESS};

pub(crate) fn extent_of(
    bounds: Rect,
    axis: Axis,
) -> f32 {
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

pub(crate) fn first_extent(
    total: f32,
    ratio: f32,
) -> f32 {
    let room = shareable(total);
    if !honours_minimum(room) {
        return room * EQUAL_SHARE;
    }
    (room * ratio).clamp(MIN_PANE_EXTENT, room - MIN_PANE_EXTENT)
}

pub(crate) fn ratio_for(
    total: f32,
    wanted: f32,
) -> Option<f32> {
    let room = shareable(total);
    honours_minimum(room).then(|| wanted.clamp(MIN_PANE_EXTENT, room - MIN_PANE_EXTENT) / room)
}
#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::split_fixtures::{bounds, rects, two_by_one};

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
}
