use std::time::{Duration, Instant};

use model::Rect;
use ui::{CubicBezier, Glide, Motion, timing};

const SPAN: Duration = timing::MENU;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct RectTween {
    from: Rect,
    to: Rect,
    progress: Glide,
}

impl RectTween {
    pub(crate) fn resting(rect: Rect) -> Self {
        Self {
            from: rect,
            to: rect,
            progress: Glide::resting(1.0, SPAN, CubicBezier::DEFAULT),
        }
    }

    pub(crate) fn go_to(&mut self, rect: Rect, now: Instant, motion: Motion) {
        if self.to == rect {
            return;
        }
        self.from = self.rect(now, motion);
        self.to = rect;
        self.progress.snap(0.0);
        self.progress.go_to(1.0, now, motion);
    }

    pub(crate) fn rect(&self, now: Instant, motion: Motion) -> Rect {
        let t = self.progress.value(now, motion);
        let mix = |a: f32, b: f32| a + (b - a) * t;
        Rect {
            x: mix(self.from.x, self.to.x),
            y: mix(self.from.y, self.to.y),
            width: mix(self.from.width, self.to.width),
            height: mix(self.from.height, self.to.height),
        }
    }

    pub(crate) fn is_running(&self, now: Instant, motion: Motion) -> bool {
        self.progress.is_running(now, motion)
    }
}

#[cfg(test)]
#[path = "rect_tween_tests.rs"]
mod tests;
