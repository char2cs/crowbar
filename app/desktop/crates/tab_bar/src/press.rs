use gpui::{Pixels, Point};

const THRESHOLD: f32 = 8.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Press {
    pub(crate) index: usize,
    pub(crate) origin: Point<Pixels>,
}

impl Press {
    pub(crate) fn has_moved_past_threshold(
        &self,
        position: Point<Pixels>,
    ) -> bool {
        let dx = f32::from(position.x - self.origin.x);
        let dy = f32::from(position.y - self.origin.y);
        dx.hypot(dy) > THRESHOLD
    }
}

#[cfg(test)]
mod tests {
    use gpui::{point, px};

    use super::*;

    #[test]
    fn a_press_becomes_a_drag_after_more_than_eight_pixels() {
        let press = Press {
            index: 0,
            origin: point(px(100.0), px(20.0)),
        };
        assert!(
            !press.has_moved_past_threshold(point(px(105.0), px(26.0))),
            "7.8 pixels"
        );
        assert!(
            !press.has_moved_past_threshold(point(px(108.0), px(20.0))),
            "exactly eight"
        );
        assert!(
            press.has_moved_past_threshold(point(px(106.0), px(27.0))),
            "9.2 pixels"
        );
    }
}
