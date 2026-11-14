//! Timing of one animated transition.

use std::time::Duration;

use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Tween {
    delay: Duration,
    span: Duration,
    curve: CubicBezier,
}

impl Tween {
    pub(crate) const fn new(span: Duration, curve: CubicBezier) -> Self {
        Self {
            delay: Duration::ZERO,
            span,
            curve,
        }
    }

    pub(crate) const fn delayed(self, delay: Duration) -> Self {
        Self { delay, ..self }
    }

    pub(crate) fn at(&self, elapsed: Duration, motion: Motion) -> (f32, bool) {
        let delay = motion.duration(self.delay).min(self.delay);
        let span = motion.duration(self.span);
        let Some(into) = elapsed.checked_sub(delay) else {
            return (0.0, true);
        };
        let progress = (into.as_secs_f32() / span.as_secs_f32()).clamp(0.0, 1.0);
        (self.curve.ease(progress), progress < 1.0)
    }
}

#[cfg(test)]
#[path = "tween_tests.rs"]
mod tests;
