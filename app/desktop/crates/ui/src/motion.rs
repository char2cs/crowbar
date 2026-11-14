//! Reduced-motion preference and the animation factory.

use std::rc::Rc;
use std::time::Duration;

use gpui::{Animation, App, Global};

use crate::cubic_bezier::CubicBezier;

/// Duration a reduced-motion animation takes: the `0.01ms` of the spec's blanket rule.
const REDUCED: Duration = Duration::from_micros(10);

/// Global motion preference; the shell sets it from the OS "reduce motion" setting.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct Motion {
    reduced: bool,
}

impl Global for Motion {}

impl Motion {
    /// Preference that honours "reduce motion".
    pub fn reduced() -> Self {
        Self { reduced: true }
    }

    /// The installed preference; full motion when none was installed.
    pub fn of(cx: &App) -> Self {
        cx.try_global::<Self>().copied().unwrap_or_default()
    }

    /// Whether animations are collapsed to an instant.
    pub fn is_reduced(self) -> bool {
        self.reduced
    }

    /// `duration`, or the instant duration when motion is reduced.
    pub fn duration(self, duration: Duration) -> Duration {
        if self.reduced { REDUCED } else { duration }
    }

    /// A one-shot animation of `duration` along `curve`, honouring the preference.
    pub fn animation(self, duration: Duration, curve: CubicBezier) -> Animation {
        Animation::new(self.duration(duration)).with_easing(Rc::new(move |t| curve.ease(t)))
    }

    /// A looping animation that stays active under reduced motion (spinners are exempt).
    pub fn essential_loop(duration: Duration, curve: CubicBezier) -> Animation {
        Animation::new(duration).repeat().with_easing(Rc::new(move |t| curve.ease(t)))
    }
}

#[cfg(test)]
#[path = "motion_tests.rs"]
mod tests;
