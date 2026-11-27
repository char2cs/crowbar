pub(crate) mod adapters;
mod cubic_bezier;
mod glide;
pub(crate) mod hover;
mod hover_fill;
mod keyframes;
mod pose;
mod position;
mod toggle_state;
mod tween;

pub use cubic_bezier::CubicBezier;
pub use glide::Glide;
pub use hover_fill::HoverFill;
pub(crate) use keyframes::Keyframes;
pub use pose::Pose;
pub(crate) use toggle_state::ToggleState;
pub(crate) use tween::Tween;

use std::time::Duration;

use gpui::{Animation, App, Global};

const REDUCED: Duration = Duration::from_micros(10);

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct Motion {
    reduced: bool,
}

impl Global for Motion {}

impl Motion {
    pub fn reduced() -> Self {
        Self { reduced: true }
    }

    pub fn of(cx: &App) -> Self {
        cx.try_global::<Self>().copied().unwrap_or_default()
    }

    pub fn is_reduced(self) -> bool {
        self.reduced
    }

    pub fn duration(
        self,
        duration: Duration,
    ) -> Duration {
        if self.reduced {
            return REDUCED;
        }
        duration
    }

    pub fn animation(
        self,
        duration: Duration,
        curve: CubicBezier,
    ) -> Animation {
        Animation::new(self.duration(duration)).with_easing(move |t| curve.ease(t))
    }

    pub fn essential_loop(
        duration: Duration,
        curve: CubicBezier,
    ) -> Animation {
        Animation::new(duration)
            .repeat()
            .with_easing(move |t| curve.ease(t))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static str, fn() -> Motion, u64, Duration);

    const CASES: &[Case] = &[
        (
            "full motion keeps the duration",
            Motion::default,
            200,
            Duration::from_millis(200),
        ),
        (
            "reduced motion collapses it",
            Motion::reduced,
            200,
            Duration::from_micros(10),
        ),
    ];

    #[test]
    fn reduced_motion_collapses_durations() {
        for &(name, motion, millis, want) in CASES {
            let duration = motion().duration(Duration::from_millis(millis));
            assert_eq!(duration, want, "{name}");
        }
        assert!(Motion::reduced().is_reduced() && !Motion::default().is_reduced());
    }

    #[test]
    fn animation_applies_the_curve() {
        let animation =
            Motion::default().animation(Duration::from_millis(150), CubicBezier::DEFAULT);
        assert_eq!(animation.duration, Duration::from_millis(150));
        assert!(animation.oneshot);
        let eased = (animation.easing)(0.5);
        assert!((eased - 0.77556).abs() < 2e-4, "{eased}");
    }

    #[test]
    fn essential_loops_ignore_reduction() {
        let animation = Motion::essential_loop(Duration::from_secs(1), CubicBezier::LINEAR);
        assert_eq!(animation.duration, Duration::from_secs(1));
        assert!(!animation.oneshot);
    }
}
