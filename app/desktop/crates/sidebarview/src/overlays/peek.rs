//! The floating card shown while a closed sidebar is peeked at.

mod exit;
mod ops;

use std::rc::Rc;
use std::time::Duration;

use crate::sidebar::Clock;
use ui::{CubicBezier, Glide, Motion};

const SLIDE: Duration = Duration::from_millis(200);
const CURVE: CubicBezier = CubicBezier::new(0.4, 0.0, 0.2, 1.0);

pub(crate) struct Peek {
    glide: Glide,
    clock: Rc<dyn Clock>,
}

impl Peek {
    pub(crate) fn new(clock: Rc<dyn Clock>) -> Self {
        Self {
            glide: Glide::resting(0.0, SLIDE, CURVE),
            clock,
        }
    }

    pub(crate) fn slide_to(
        &mut self,
        target: f32,
        motion: Motion,
    ) {
        self.glide.go_to(target, self.clock.now(), motion);
    }

    pub(crate) fn position(
        &self,
        motion: Motion,
    ) -> f32 {
        self.glide.value(self.clock.now(), motion)
    }

    pub(crate) fn is_moving(
        &self,
        motion: Motion,
    ) -> bool {
        self.glide.is_running(self.clock.now(), motion)
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

impl std::fmt::Debug for Peek {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Peek")
            .field("glide", &self.glide)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::time::Instant;

    use super::*;

    fn peek_at(reading: &Rc<Cell<Instant>>) -> Peek {
        let seen = Rc::clone(reading);
        Peek::new(Rc::new(move || seen.get()))
    }

    #[test]
    fn sliding_in_takes_two_hundred_milliseconds_and_reduced_motion_is_instant() {
        let reading = Rc::new(Cell::new(Instant::now()));
        let mut peek = peek_at(&reading);
        assert!(!peek.is_wanted(), "parked at start");
        peek.slide_to(1.0, Motion::default());
        assert!(peek.is_wanted(), "wanted after slide in");
        assert!(
            peek.is_moving(Motion::default()),
            "moving right after the request"
        );
        assert!(peek.position(Motion::default()) < 0.1, "barely started");
        reading.set(reading.get() + SLIDE);
        assert!(!peek.is_moving(Motion::default()), "arrived after the span");
        assert!(peek.position(Motion::default()) > 0.9, "fully shown");
        let mut instant = peek_at(&reading);
        instant.slide_to(1.0, Motion::reduced());
        assert!(instant.position(Motion::reduced()) < 0.1, "starts at zero");
    }
}
