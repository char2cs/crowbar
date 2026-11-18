//! The movement a pressed row must make before it becomes a drag.

const DRAG_THRESHOLD: f32 = 8.0;

/// A row pressed at a point, waiting to move far enough to become a drag.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct DragArm {
    origin: (f32, f32),
}

impl DragArm {
    /// A row pressed at `origin`.
    pub fn at(origin: (f32, f32)) -> Self {
        Self { origin }
    }

    /// Whether the pointer, now at `now`, has moved far enough from the press (8 px, measured as
    /// a straight line) to start a drag.
    pub fn started(
        self,
        now: (f32, f32),
    ) -> bool {
        let (dx, dy) = (now.0 - self.origin.0, now.1 - self.origin.1);
        dx.hypot(dy) >= DRAG_THRESHOLD
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_threshold_is_euclidean() {
        assert!(
            !DragArm::at((0.0, 0.0)).started((5.0, 5.0)),
            "7.07px is not enough"
        );
        assert!(DragArm::at((0.0, 0.0)).started((6.0, 6.0)), "8.49px starts");
        assert!(
            DragArm::at((10.0, 10.0)).started((10.0, 18.0)),
            "exactly 8px starts"
        );
    }
}
