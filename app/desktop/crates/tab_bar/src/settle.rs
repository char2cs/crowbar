use std::time::{Duration, Instant};

use model::TabId;
use ui::{CubicBezier, Glide, Motion};

const SPAN: Duration = Duration::from_millis(150);

#[derive(Debug)]
pub(crate) struct Settle {
    pub(crate) tab: TabId,
    from: f32,
    glide: Glide,
}

impl Settle {
    pub(crate) fn new(tab: TabId, from: f32, now: Instant, motion: Motion) -> Self {
        let mut glide = Glide::resting(1.0, SPAN, CubicBezier::EASE_OUT);
        glide.snap(1.0);
        glide.go_to(0.0, now, motion);
        Self { tab, from, glide }
    }

    pub(crate) fn offset(&self, now: Instant, motion: Motion) -> f32 {
        self.from * self.glide.value(now, motion)
    }

    pub(crate) fn is_running(&self, now: Instant, motion: Motion) -> bool {
        self.glide.is_running(now, motion)
    }
}

#[cfg(test)]
#[path = "settle_tests.rs"]
mod tests;
