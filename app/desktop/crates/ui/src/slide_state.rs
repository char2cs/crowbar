//! Position of a sliding indicator between slots.

use std::time::{Duration, Instant};

use gpui::Context;

use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;

/// The slot an indicator is moving away from, the one it is moving to, and when it started.
#[derive(Debug)]
pub(crate) struct SlideState {
    from: f32,
    to: usize,
    started: Option<Instant>,
}

impl SlideState {
    pub(crate) fn starting(slot: usize) -> Self {
        Self { from: slot as f32, to: slot, started: None }
    }

    /// Retargets the indicator; a running slide continues from where it currently is.
    pub(crate) fn go_to(&mut self, slot: usize, span: Duration, curve: CubicBezier, motion: Motion, cx: &mut Context<'_, Self>) {
        if self.to == slot {
            return;
        }
        self.from = self.at(span, curve, motion, Instant::now()).0;
        self.to = slot;
        self.started = Some(Instant::now());
        cx.notify();
    }

    /// Fractional slot at `now` and whether the slide is still running.
    pub(crate) fn at(&self, span: Duration, curve: CubicBezier, motion: Motion, now: Instant) -> (f32, bool) {
        let target = self.to as f32;
        let Some(started) = self.started else {
            return (target, false);
        };
        let span = motion.duration(span);
        let progress = (now.saturating_duration_since(started).as_secs_f32() / span.as_secs_f32()).clamp(0.0, 1.0);
        (self.from + (target - self.from) * curve.ease(progress), progress < 1.0)
    }
}

#[cfg(test)]
#[path = "slide_state_tests.rs"]
mod tests;
