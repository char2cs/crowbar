//! Position of a sliding indicator between slots.

use std::time::Instant;

use gpui::Context;

use crate::motion::Motion;
use crate::tween::Tween;

#[derive(Debug)]
pub(crate) struct SlideState {
    from: f32,
    to: u16,
    started: Option<Instant>,
}

impl SlideState {
    pub(crate) fn starting(slot: u16) -> Self {
        Self {
            from: f32::from(slot),
            to: slot,
            started: None,
        }
    }

    pub(crate) fn go_to(&mut self, slot: u16, tween: Tween, cx: &mut Context<'_, Self>) {
        if self.to == slot {
            return;
        }
        let now = Instant::now();
        self.from = self.at(tween, Motion::of(cx), now).0;
        self.to = slot;
        self.started = Some(now);
        cx.notify();
    }

    pub(crate) fn at(&self, tween: Tween, motion: Motion, now: Instant) -> (f32, bool) {
        let target = f32::from(self.to);
        let Some(started) = self.started else {
            return (target, false);
        };
        let (eased, running) = tween.at(now.saturating_duration_since(started), motion);
        (self.from + (target - self.from) * eased, running)
    }
}

#[cfg(test)]
#[path = "slide_state_tests.rs"]
mod tests;
