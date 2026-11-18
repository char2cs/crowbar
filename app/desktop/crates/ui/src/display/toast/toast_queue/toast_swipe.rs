//! A pointer drag that may dismiss a toast.

use gpui::{Pixels, Point, px};

use crate::display::toast::toast_queue::toast_id::ToastId;
use crate::display::toast::toast_queue::toast_side::ToastSide;

const THRESHOLD: f32 = 40.0;

#[derive(Debug, Clone, Copy)]
pub(crate) struct ToastSwipe {
    pub(crate) toast: ToastId,
    pub(crate) origin: Point<Pixels>,
}

impl ToastSwipe {
    pub(crate) fn dismisses(
        &self,
        position: Point<Pixels>,
        side: ToastSide,
    ) -> bool {
        let moved = position - self.origin;
        let sideways = match side {
            ToastSide::Left => -moved.x,
            ToastSide::Right => moved.x,
        };
        sideways >= px(THRESHOLD) || moved.y >= px(THRESHOLD)
    }
}

#[cfg(test)]
mod tests {
    use gpui::point;

    use super::*;

    #[test]
    fn a_swipe_toward_the_edge_or_down_dismisses_past_forty_pixels() {
        type Case = (ToastSide, f32, f32, bool);
        const CASES: &[Case] = &[
            (ToastSide::Left, -40.0, 0.0, true),
            (ToastSide::Left, -39.0, 0.0, false),
            (ToastSide::Left, 80.0, 0.0, false),
            (ToastSide::Right, 40.0, 0.0, true),
            (ToastSide::Right, -80.0, 0.0, false),
            (ToastSide::Left, 0.0, 40.0, true),
            (ToastSide::Right, 0.0, -80.0, false),
        ];
        let swipe = ToastSwipe {
            toast: ToastId(1),
            origin: point(px(100.0), px(100.0)),
        };
        for &(side, dx, dy, dismissed) in CASES {
            let at = point(px(100.0 + dx), px(100.0 + dy));
            assert_eq!(swipe.dismisses(at, side), dismissed, "{side:?} {dx},{dy}");
        }
    }
}
