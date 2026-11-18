//! Timing of one animated transition.

use std::time::Duration;

use crate::foundation::motion::Motion;
use crate::foundation::motion::cubic_bezier::CubicBezier;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Tween {
    delay: Duration,
    span: Duration,
    curve: CubicBezier,
}

impl Tween {
    pub(crate) const fn new(
        span: Duration,
        curve: CubicBezier,
    ) -> Self {
        Self {
            delay: Duration::ZERO,
            span,
            curve,
        }
    }

    pub(crate) const fn delayed(
        self,
        delay: Duration,
    ) -> Self {
        Self { delay, ..self }
    }

    pub(crate) fn at(
        &self,
        elapsed: Duration,
        motion: Motion,
    ) -> (f32, bool) {
        if motion.is_reduced() {
            return (self.curve.ease(1.0), false);
        }
        let Some(into) = elapsed.checked_sub(self.delay) else {
            return (0.0, true);
        };
        let progress = (into.as_secs_f32() / self.span.as_secs_f32()).clamp(0.0, 1.0);
        (self.curve.ease(progress), progress < 1.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const SPAN: Duration = Duration::from_millis(100);

    type Case = (&'static str, u64, u64, fn() -> Motion, (f32, bool, f32));

    const CASES: &[Case] = &[
        (
            "progress starts at zero",
            0,
            0,
            Motion::default,
            (0.0, true, 1e-6),
        ),
        (
            "progress follows the curve",
            0,
            50,
            Motion::default,
            (0.5, true, 1e-3),
        ),
        (
            "progress settles",
            0,
            250,
            Motion::default,
            (1.0, false, 0.0),
        ),
        (
            "a delay holds progress at zero",
            40,
            39,
            Motion::default,
            (0.0, true, 0.0),
        ),
        (
            "progress resumes after the delay",
            40,
            90,
            Motion::default,
            (0.5, true, 1e-3),
        ),
        (
            "reduced motion finishes at once",
            40,
            1,
            Motion::reduced,
            (1.0, false, 0.0),
        ),
        (
            "reduced motion finishes before any time has passed",
            40,
            0,
            Motion::reduced,
            (1.0, false, 0.0),
        ),
    ];

    #[test]
    fn progress_follows_the_curve_the_delay_and_the_motion_preference() {
        for &(name, delay_ms, elapsed_ms, motion, (progress, running, tolerance)) in CASES {
            let tween =
                Tween::new(SPAN, CubicBezier::LINEAR).delayed(Duration::from_millis(delay_ms));
            let at = tween.at(Duration::from_millis(elapsed_ms), motion());
            assert!(
                (at.0 - progress).abs() <= tolerance,
                "{name}: progress {}",
                at.0
            );
            assert_eq!(at.1, running, "{name}: running");
        }
    }
}
