//! The movement a pressed row must make before it becomes a drag.

const DRAG_THRESHOLD: f32 = 8.0;

pub(crate) fn drag_started(
    origin: (f32, f32),
    now: (f32, f32),
) -> bool {
    let (dx, dy) = (now.0 - origin.0, now.1 - origin.1);
    dx.hypot(dy) >= DRAG_THRESHOLD
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_threshold_is_euclidean() {
        assert!(
            !drag_started((0.0, 0.0), (5.0, 5.0)),
            "7.07px is not enough"
        );
        assert!(drag_started((0.0, 0.0), (6.0, 6.0)), "8.49px starts");
        assert!(
            drag_started((10.0, 10.0), (10.0, 18.0)),
            "exactly 8px starts"
        );
    }
}
