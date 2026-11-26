//! The movement a pressed row must make before it becomes a drag.

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/components/sidebar/hooks/use-sidebar-drag.ts
const ROW_THRESHOLD: f32 = 8.0;
// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/file-explorer/file-explorer/hooks/use-tree-container-events.ts
const FILE_THRESHOLD: f32 = 5.0;

/// A row pressed at a point, waiting to move far enough to become a drag.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct DragArm {
    origin: (f32, f32),
    threshold: f32,
}

impl DragArm {
    /// A chat-tree row pressed at `origin`.
    pub fn at(origin: (f32, f32)) -> Self {
        Self {
            origin,
            threshold: ROW_THRESHOLD,
        }
    }

    /// A Files row pressed at `origin`, which starts its drag sooner.
    pub fn file_at(origin: (f32, f32)) -> Self {
        Self {
            origin,
            threshold: FILE_THRESHOLD,
        }
    }

    /// Whether the pointer, now at `now`, has moved far enough from the press (8 px for a chat
    /// row, 5 px for a file, measured as a straight line) to start a drag.
    pub fn started(
        self,
        now: (f32, f32),
    ) -> bool {
        let (dx, dy) = (now.0 - self.origin.0, now.1 - self.origin.1);
        dx.hypot(dy) >= self.threshold
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (fn((f32, f32)) -> DragArm, (f32, f32), bool, &'static str);

    #[test]
    fn the_threshold_is_euclidean_and_shorter_for_a_file() {
        const CASES: &[Case] = &[
            (DragArm::at, (5.0, 5.0), false, "7.07px is not enough"),
            (DragArm::at, (6.0, 6.0), true, "8.49px starts"),
            (DragArm::at, (0.0, 8.0), true, "exactly 8px starts"),
            (DragArm::file_at, (3.0, 3.0), false, "4.24px is not enough"),
            (DragArm::file_at, (0.0, 5.0), true, "a file starts at 5px"),
        ];
        for (arm, now, started, case) in CASES {
            assert_eq!(arm((0.0, 0.0)).started(*now), *started, "{case}");
        }
    }
}
