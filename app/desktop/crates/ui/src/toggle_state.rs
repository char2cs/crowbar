//! A two-valued state that remembers when it last flipped, to animate between its values.

use std::time::Instant;

use gpui::Context;

use crate::motion::Motion;
use crate::pose::Pose;
use crate::position::Position;
use crate::tween::Tween;

#[derive(Debug, Default)]
pub(crate) struct ToggleState {
    pose: Pose,
    changed: Option<Instant>,
}

impl ToggleState {
    pub(crate) fn starting(pose: Pose) -> Self {
        Self {
            pose,
            changed: None,
        }
    }

    pub(crate) fn set(&mut self, pose: Pose, cx: &mut Context<'_, Self>) {
        if self.pose == pose {
            return;
        }
        self.pose = pose;
        self.changed = Some(Instant::now());
        cx.notify();
    }

    pub(crate) fn entered(&self, tween: Tween, motion: Motion, now: Instant) -> Position {
        let Some(changed) = self.changed.filter(|_| self.pose == Pose::On) else {
            return Position::settled(self.pose.rest());
        };
        let (value, running) = tween.at(now.saturating_duration_since(changed), motion);
        Position { value, running }
    }

    pub(crate) fn position(&self, tween: Tween, motion: Motion, now: Instant) -> Position {
        let Some(changed) = self.changed else {
            return Position::settled(self.pose.rest());
        };
        let (eased, running) = tween.at(now.saturating_duration_since(changed), motion);
        let target = self.pose.rest();
        let origin = 1.0 - target;
        Position {
            value: origin + (target - origin) * eased,
            running,
        }
    }
}

#[cfg(test)]
#[path = "toggle_state_tests.rs"]
mod tests;
