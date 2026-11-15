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
    pub fn resting(
        value: f32,
        span: Duration,
        curve: CubicBezier,
    ) -> Self {
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
    pub fn go_to(
        &mut self,
        target: f32,
        now: Instant,
        motion: Motion,
    ) {
        if (self.to - target).abs() < f32::EPSILON {
            return;
        }
        self.from = self.value(now, motion);
        self.to = target;
        self.started = Some(now);
    }

    /// Jumps to `value` without animating.
    pub fn snap(
        &mut self,
        value: f32,
    ) {
        self.from = value;
        self.to = value;
        self.started = None;
    }

    /// The value at `now`.
    pub fn value(
        &self,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        self.from + (self.to - self.from) * self.progress(now, motion)
    }

    /// Whether the value is still on its way at `now`.
    pub fn is_running(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.started
            .is_some_and(|started| self.elapsed_fraction(started, now, motion) < 1.0)
    }

    fn progress(
        &self,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        let Some(started) = self.started else {
            return 1.0;
        };
        self.curve.ease(self.elapsed_fraction(started, now, motion))
    }

    fn elapsed_fraction(
        &self,
        started: Instant,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        let span = motion.duration(self.span).as_secs_f32();
        let elapsed = now.saturating_duration_since(started).as_secs_f32();
        (elapsed / span).clamp(0.0, 1.0)
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use super::*;

    const SPAN: Duration = Duration::from_millis(100);

    type Moves = &'static [(f32, u64)];
    type Expect = (f32, f32, bool);

    type Case = (&'static str, f32, Moves, fn() -> Motion, u64, Expect);

    const CASES: &[Case] = &[
        (
            "rest holds its value",
            3.0,
            &[],
            Motion::default,
            0,
            (3.0, 3.0, false),
        ),
        (
            "halfway",
            0.0,
            &[(10.0, 0)],
            Motion::default,
            50,
            (5.0, 10.0, true),
        ),
        (
            "arrival",
            0.0,
            &[(10.0, 0)],
            Motion::default,
            100,
            (10.0, 10.0, false),
        ),
        (
            "retargeting starts from the value shown",
            0.0,
            &[(10.0, 0), (0.0, 50)],
            Motion::default,
            50,
            (5.0, 0.0, true),
        ),
        (
            "the same target keeps its course",
            0.0,
            &[(10.0, 0), (10.0, 50)],
            Motion::default,
            50,
            (5.0, 10.0, true),
        ),
        (
            "reduced motion arrives at once",
            0.0,
            &[(10.0, 0)],
            Motion::reduced,
            1,
            (10.0, 10.0, false),
        ),
    ];

    fn glide(value: f32) -> Glide {
        Glide::resting(value, SPAN, CubicBezier::LINEAR)
    }

    fn played(
        mut moving: Glide,
        moves: Moves,
        (start, motion): (Instant, Motion),
    ) -> Glide {
        for &(to, at_ms) in moves {
            moving.go_to(to, start + Duration::from_millis(at_ms), motion);
        }
        moving
    }

    #[test]
    fn moves_reach_the_expected_value_target_and_state() {
        for &(name, from, moves, motion, probe_ms, (value, target, running)) in CASES {
            let (start, motion) = (Instant::now(), motion());
            let moving = played(glide(from), moves, (start, motion));
            let probe = start + Duration::from_millis(probe_ms);
            assert!(
                (moving.value(probe, motion) - value).abs() < 1e-3,
                "{name}: value"
            );
            assert!((moving.target() - target).abs() < 1e-3, "{name}: target");
            assert_eq!(moving.is_running(probe, motion), running, "{name}: running");
        }
    }

    #[test]
    fn snap_jumps_without_motion() {
        let now = Instant::now();
        let mut moving = glide(0.0);
        moving.go_to(10.0, now, Motion::default());
        moving.snap(2.0);
        assert_eq!(moving.value(now, Motion::default()), 2.0, "snapped value");
        assert!(
            !moving.is_running(now, Motion::default()),
            "snap stops motion"
        );
    }
}
