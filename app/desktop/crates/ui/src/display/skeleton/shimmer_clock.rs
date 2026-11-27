use std::time::{Duration, Instant};

use gpui::{App, Global};

use crate::foundation::motion::adapters::clock as system_clock;
use crate::foundation::tokens::timing;

const HEAD_START: f32 = 0.5;

#[derive(Debug, Clone, Copy)]
pub(crate) struct ShimmerClock {
    start: Instant,
}

impl Global for ShimmerClock {}

impl ShimmerClock {
    pub(crate) fn starting_now() -> Self {
        Self {
            start: system_clock::now(),
        }
    }

    pub(crate) fn of(cx: &App) -> Self {
        cx.try_global::<Self>()
            .copied()
            .unwrap_or_else(Self::starting_now)
    }

    pub(crate) fn phase(&self) -> f32 {
        phase_after(self.start.elapsed())
    }
}

pub(crate) fn phase_after(elapsed: Duration) -> f32 {
    let cycles = elapsed.as_secs_f32() / timing::SKELETON.as_secs_f32();
    (cycles + HEAD_START).fract()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_shimmer_starts_half_way_through_its_cycle() {
        const CASES: &[(u64, f32)] = &[
            (0, 0.5),
            (500, 0.75),
            (1000, 0.0),
            (2000, 0.5),
            (3500, 0.25),
        ];
        for &(millis, phase) in CASES {
            let got = phase_after(Duration::from_millis(millis));
            assert!((got - phase).abs() < 1e-4, "{millis}ms: {got}");
        }
    }
}
