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

    pub(crate) fn set(
        &mut self,
        pose: Pose,
        cx: &mut Context<'_, Self>,
    ) {
        if self.pose == pose {
            return;
        }
        self.pose = pose;
        self.changed = Some(Instant::now());
        cx.notify();
    }

    pub(crate) fn entered(
        &self,
        tween: Tween,
        motion: Motion,
        now: Instant,
    ) -> Position {
        let Some(changed) = self.changed.filter(|_| self.pose == Pose::On) else {
            return Position::settled(self.pose.rest());
        };
        let (value, running) = tween.at(now.saturating_duration_since(changed), motion);
        Position { value, running }
    }

    pub(crate) fn position(
        &self,
        tween: Tween,
        motion: Motion,
        now: Instant,
    ) -> Position {
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
mod tests {
    use std::time::Duration;

    use gpui::{AppContext as _, Entity, TestAppContext};

    use super::*;
    use crate::cubic_bezier::CubicBezier;

    const SPAN: Duration = Duration::from_millis(150);

    fn tween() -> Tween {
        Tween::new(SPAN, CubicBezier::LINEAR)
    }

    fn position(
        state: &ToggleState,
        after: Duration,
        start: Instant,
    ) -> Position {
        state.position(tween(), Motion::default(), start + after)
    }

    fn entered(
        state: &Entity<ToggleState>,
        delayed: Tween,
        moment: Instant,
        cx: &TestAppContext,
    ) -> Position {
        state.read_with(cx, |s, _| s.entered(delayed, Motion::default(), moment))
    }

    #[gpui::test]
    fn starting_value_does_not_animate(cx: &mut TestAppContext) {
        let state = cx.new(|_| ToggleState::starting(Pose::On));
        let at = state.read_with(cx, |s, _| position(s, Duration::ZERO, Instant::now()));
        assert_eq!(at, Position::settled(1.0));
    }

    #[gpui::test]
    fn flipping_eases_between_the_ends(cx: &mut TestAppContext) {
        let state = cx.new(|_| ToggleState::starting(Pose::Off));
        state.update(cx, |s, cx| s.set(Pose::On, cx));
        let start = Instant::now();
        let (early, mid, late) = state.read_with(cx, |s, _| {
            (
                position(s, Duration::ZERO, start),
                position(s, Duration::from_millis(75), start),
                position(s, Duration::from_millis(300), start),
            )
        });
        assert!(early.running && early.value < 0.2, "{early:?}");
        assert!(mid.running && (mid.value - 0.5).abs() < 0.15, "{mid:?}");
        assert_eq!(late, Position::settled(1.0));
    }

    #[gpui::test]
    fn flipping_back_eases_down(cx: &mut TestAppContext) {
        let state = cx.new(|_| ToggleState::starting(Pose::On));
        state.update(cx, |s, cx| s.set(Pose::Off, cx));
        let start = Instant::now();
        let mid = state.read_with(cx, |s, _| position(s, Duration::from_millis(75), start));
        assert!(mid.running && (mid.value - 0.5).abs() < 0.15, "{mid:?}");
    }

    #[gpui::test]
    fn reduced_motion_jumps_to_the_target(cx: &mut TestAppContext) {
        let state = cx.new(|_| ToggleState::starting(Pose::Off));
        state.update(cx, |s, cx| s.set(Pose::On, cx));
        let later = Instant::now() + Duration::from_millis(1);
        let at = state.read_with(cx, |s, _| s.position(tween(), Motion::reduced(), later));
        assert_eq!(at, Position::settled(1.0));
    }

    #[gpui::test]
    fn setting_the_same_value_keeps_it_idle(cx: &mut TestAppContext) {
        let state = cx.new(|_| ToggleState::starting(Pose::On));
        state.update(cx, |s, cx| s.set(Pose::On, cx));
        let at = state.read_with(cx, |s, _| position(s, Duration::ZERO, Instant::now()));
        assert!(!at.running);
    }

    #[gpui::test]
    fn entrance_waits_then_eases(cx: &mut TestAppContext) {
        let state = cx.new(|_| ToggleState::default());
        state.update(cx, |s, cx| s.set(Pose::On, cx));
        let start = Instant::now();
        let delayed = tween().delayed(SPAN);
        let at = |after_ms: u64| {
            let moment = start + Duration::from_millis(after_ms);
            entered(&state, delayed, moment, cx)
        };
        assert_eq!(
            at(100),
            Position {
                value: 0.0,
                running: true
            }
        );
        let mid = at(225);
        assert!(mid.running && (mid.value - 0.5).abs() < 0.1, "{mid:?}");
        assert_eq!(at(400), Position::settled(1.0));
    }

    #[gpui::test]
    fn entrance_is_zero_while_off(cx: &mut TestAppContext) {
        let state = cx.new(|_| ToggleState::default());
        let at = state.read_with(cx, |s, _| {
            s.entered(tween(), Motion::default(), Instant::now())
        });
        assert_eq!(at, Position::settled(0.0));
    }
}
