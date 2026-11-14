//! A number that glides to a new value over a fixed time.

use std::time::{Duration, Instant};

use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;

/// A value that moves from where it is to a new target along an easing curve; ask it for the value
/// at a point in time and whether it is still moving. Retargeting mid-flight starts from the
/// current value, so nothing jumps.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Glide {
    from: f32,
    to: f32,
    started: Option<Instant>,
    span: Duration,
    curve: CubicBezier,
}

impl Glide {
    /// A glide resting at `value` that takes `span` along `curve` for every later move.
    pub fn resting(value: f32, span: Duration, curve: CubicBezier) -> Self {
        Self {
            from: value,
            to: value,
            started: None,
            span,
            curve,
        }
    }

    /// The target the glide is heading for.
    pub fn target(&self) -> f32 {
        self.to
    }

    /// Starts moving toward `target` from the value shown at `now`; the same target is a no-op.
    pub fn go_to(&mut self, target: f32, now: Instant, motion: Motion) {
        if (self.to - target).abs() < f32::EPSILON {
            return;
        }
        self.from = self.value(now, motion);
        self.to = target;
        self.started = Some(now);
    }

    /// Jumps to `value` without animating.
    pub fn snap(&mut self, value: f32) {
        self.from = value;
        self.to = value;
        self.started = None;
    }

    /// The value at `now`.
    pub fn value(&self, now: Instant, motion: Motion) -> f32 {
        self.from + (self.to - self.from) * self.progress(now, motion)
    }

    /// Whether the value is still on its way at `now`.
    pub fn is_running(&self, now: Instant, motion: Motion) -> bool {
        self.started
            .is_some_and(|started| self.elapsed_fraction(started, now, motion) < 1.0)
    }

    fn progress(&self, now: Instant, motion: Motion) -> f32 {
        let Some(started) = self.started else {
            return 1.0;
        };
        self.curve.ease(self.elapsed_fraction(started, now, motion))
    }

    fn elapsed_fraction(&self, started: Instant, now: Instant, motion: Motion) -> f32 {
        let span = motion.duration(self.span).as_secs_f32();
        let elapsed = now.saturating_duration_since(started).as_secs_f32();
        (elapsed / span).clamp(0.0, 1.0)
    }
}

#[cfg(test)]
#[path = "glide_tests.rs"]
mod tests;
