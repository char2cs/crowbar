//! The floating card shown while a closed sidebar is peeked at.

use std::time::{Duration, Instant};

use ui::{CubicBezier, Glide, Motion};

const SLIDE: Duration = Duration::from_millis(200);
const CURVE: CubicBezier = CubicBezier::new(0.4, 0.0, 0.2, 1.0);

#[derive(Debug)]
pub(crate) struct Peek {
    glide: Glide,
}

impl Default for Peek {
    fn default() -> Self {
        Self {
            glide: Glide::resting(0.0, SLIDE, CURVE),
        }
    }
}

impl Peek {
    pub(crate) fn slide_to(
        &mut self,
        target: f32,
        motion: Motion,
    ) {
        self.glide.go_to(target, Instant::now(), motion);
    }

    pub(crate) fn position(
        &self,
        motion: Motion,
    ) -> f32 {
        self.glide.value(Instant::now(), motion)
    }

    pub(crate) fn is_moving(
        &self,
        motion: Motion,
    ) -> bool {
        self.glide.is_running(Instant::now(), motion)
    }

    pub(crate) fn is_shown(
        &self,
        motion: Motion,
    ) -> bool {
        self.is_wanted() || self.is_moving(motion)
    }

    pub(crate) fn is_wanted(&self) -> bool {
        self.glide.target() > 0.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sliding_in_takes_two_hundred_milliseconds_and_reduced_motion_is_instant() {
        let mut peek = Peek::default();
        assert!(!peek.is_wanted(), "parked at start");
        peek.slide_to(1.0, Motion::default());
        assert!(peek.is_wanted(), "wanted after slide in");
        assert!(
            peek.is_moving(Motion::default()),
            "moving right after the request"
        );
        assert!(peek.position(Motion::default()) < 0.1, "barely started");
        let mut instant = Peek::default();
        instant.slide_to(1.0, Motion::reduced());
        assert!(instant.position(Motion::reduced()) < 0.1, "starts at zero");
    }
}
