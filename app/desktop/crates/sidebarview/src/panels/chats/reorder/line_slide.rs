use std::time::{Duration, Instant};

use ui::{CubicBezier, Glide, Motion};

use crate::panels::chats::reorder::line_box::LineBox;

const SPAN: Duration = Duration::from_millis(60);

#[derive(Debug, Clone, Copy)]
pub(crate) struct LineSlide {
    left: Glide,
    top: Glide,
    width: Glide,
}

impl LineSlide {
    pub(crate) fn resting(at: LineBox) -> Self {
        let rest = |value| Glide::resting(value, SPAN, CubicBezier::EASE_OUT);
        Self {
            left: rest(at.left),
            top: rest(at.top),
            width: rest(at.width),
        }
    }

    pub(crate) fn slide_to(
        &mut self,
        at: LineBox,
        (now, motion): (Instant, Motion),
    ) {
        self.left.go_to(at.left, now, motion);
        self.top.go_to(at.top, now, motion);
        self.width.go_to(at.width, now, motion);
    }

    pub(crate) fn jump_to(
        &mut self,
        at: LineBox,
    ) {
        *self = Self::resting(at);
    }

    pub(crate) fn target(&self) -> LineBox {
        LineBox {
            left: self.left.target(),
            top: self.top.target(),
            width: self.width.target(),
        }
    }

    pub(crate) fn at(
        &self,
        (now, motion): (Instant, Motion),
    ) -> LineBox {
        LineBox {
            left: self.left.value(now, motion),
            top: self.top.value(now, motion),
            width: self.width.value(now, motion),
        }
    }

    pub(crate) fn is_running(
        &self,
        (now, motion): (Instant, Motion),
    ) -> bool {
        [self.left, self.top, self.width]
            .iter()
            .any(|glide| glide.is_running(now, motion))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const A: LineBox = LineBox {
        left: 8.0,
        top: 10.0,
        width: 100.0,
    };
    const B: LineBox = LineBox {
        left: 22.0,
        top: 48.0,
        width: 86.0,
    };

    #[test]
    fn a_slide_travels_to_the_new_slot_and_then_rests() {
        let start = Instant::now();
        let full = Motion::default();
        let mut slide = LineSlide::resting(A);
        slide.slide_to(B, (start, full));
        assert!(slide.is_running((start, full)), "moving");
        assert_eq!(slide.at((start, full)), A, "starts where it was");
        let later = start + SPAN * 2;
        assert!(!slide.is_running((later, full)), "arrived");
        assert_eq!(slide.at((later, full)), B, "ends on the target");
    }

    #[test]
    fn reduced_motion_and_jumps_arrive_at_once() {
        let start = Instant::now();
        let mut slide = LineSlide::resting(A);
        slide.slide_to(B, (start, Motion::reduced()));
        let soon = start + Duration::from_millis(1);
        assert_eq!(slide.at((soon, Motion::reduced())), B, "reduced");
        slide.jump_to(A);
        assert_eq!(slide.at((start, Motion::default())), A, "jump");
    }
}
