//! A two-valued state that remembers when it last flipped, to animate between its values.

use std::time::{Duration, Instant};

use gpui::Context;

use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;

/// Whether a control is on (hovered, checked) and since when.
#[derive(Debug, Default)]
pub(crate) struct ToggleState {
    on: bool,
    changed: Option<Instant>,
}

/// Position of an animated property between its off value (0) and on value (1).
#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Position {
    pub(crate) value: f32,
    pub(crate) running: bool,
}

impl ToggleState {
    /// A state that starts as `on` without animating.
    pub(crate) fn starting(on: bool) -> Self {
        Self { on, changed: None }
    }

    /// Records the new value; a no-op when it did not change.
    pub(crate) fn set(&mut self, on: bool, cx: &mut Context<'_, Self>) {
        if self.on == on {
            return;
        }
        self.on = on;
        self.changed = Some(Instant::now());
        cx.notify();
    }

    /// Progress of an entrance that starts `delay` after the state turned on and eases over `span`.
    ///
    /// Zero while off or still waiting; `running` is true while waiting or easing.
    pub(crate) fn entered(&self, delay: Duration, span: Duration, curve: CubicBezier, motion: Motion, now: Instant) -> Position {
        let Some(changed) = self.changed.filter(|_| self.on) else {
            return Position { value: if self.on { 1.0 } else { 0.0 }, running: false };
        };
        let wait = motion.duration(delay).min(delay);
        let span = motion.duration(span);
        let elapsed = now.saturating_duration_since(changed);
        let Some(into) = elapsed.checked_sub(wait) else {
            return Position { value: 0.0, running: true };
        };
        let progress = (into.as_secs_f32() / span.as_secs_f32()).clamp(0.0, 1.0);
        Position { value: curve.ease(progress), running: progress < 1.0 }
    }

    /// Where the property is at `now` while easing over `span` along `curve`.
    pub(crate) fn position(&self, span: Duration, curve: CubicBezier, motion: Motion, now: Instant) -> Position {
        let target = if self.on { 1.0 } else { 0.0 };
        let Some(changed) = self.changed else {
            return Position { value: target, running: false };
        };
        let span = motion.duration(span);
        let progress = (now.saturating_duration_since(changed).as_secs_f32() / span.as_secs_f32()).clamp(0.0, 1.0);
        let eased = curve.ease(progress);
        Position {
            value: (1.0 - target) + (target - (1.0 - target)) * eased,
            running: progress < 1.0,
        }
    }
}

#[cfg(test)]
#[path = "toggle_state_tests.rs"]
mod tests;
