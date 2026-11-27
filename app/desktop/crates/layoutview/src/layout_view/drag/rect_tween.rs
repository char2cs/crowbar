use std::time::{Duration, Instant};

use layout::Rect;
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

    pub(crate) fn go_to(
        &mut self,
        rect: Rect,
        now: Instant,
        motion: Motion,
    ) {
        if self.to == rect {
            return;
        }
        self.from = self.rect(now, motion);
        self.to = rect;
        self.progress.snap(0.0);
        self.progress.go_to(1.0, now, motion);
    }

    pub(crate) fn rect(
        &self,
        now: Instant,
        motion: Motion,
    ) -> Rect {
        let t = self.progress.value(now, motion);
        let mix = |a: f32, b: f32| a + (b - a) * t;
        Rect {
            x: mix(self.from.x, self.to.x),
            y: mix(self.from.y, self.to.y),
            width: mix(self.from.width, self.to.width),
            height: mix(self.from.height, self.to.height),
        }
    }

    pub(crate) fn is_running(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.progress.is_running(now, motion)
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use layout::Rect;
    use ui::Motion;

    use super::*;

    const A: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 100.0,
        height: 100.0,
    };
    const B: Rect = Rect {
        x: 50.0,
        y: 0.0,
        width: 50.0,
        height: 100.0,
    };

    #[test]
    fn the_rectangle_ends_exactly_on_the_target_after_a_hundred_milliseconds() {
        let start = Instant::now();
        let mut tween = RectTween::resting(A);
        tween.go_to(B, start, Motion::default());
        let end = start + Duration::from_millis(100);
        assert_eq!(tween.rect(end, Motion::default()), B, "arrived");
        assert!(!tween.is_running(end, Motion::default()), "stopped");
    }

    #[test]
    fn it_is_between_the_two_rectangles_while_moving() {
        let start = Instant::now();
        let mut tween = RectTween::resting(A);
        tween.go_to(B, start, Motion::default());
        let middle = tween.rect(start + Duration::from_millis(50), Motion::default());
        assert!(
            middle.x > 0.0 && middle.x < 50.0,
            "x in between: {}",
            middle.x
        );
        assert!(
            tween.is_running(start + Duration::from_millis(50), Motion::default()),
            "running"
        );
    }
}
