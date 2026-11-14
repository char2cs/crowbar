//! The movement a pressed row must make before it becomes a drag.

use crate::metrics::DRAG_THRESHOLD;

/// Whether a pointer at `now` has left the 8px circle around `origin`.
pub fn drag_started(origin: (f32, f32), now: (f32, f32)) -> bool {
    let (dx, dy) = (now.0 - origin.0, now.1 - origin.1);
    dx.hypot(dy) >= DRAG_THRESHOLD
}

#[cfg(test)]
#[path = "drag_arm_tests.rs"]
mod tests;
