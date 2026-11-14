//! Geometry of the scrollbar thumb along one axis.

const MARGIN: f32 = 4.0;
const MIN_LENGTH: f32 = 24.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Thumb {
    pub(crate) start: f32,
    pub(crate) length: f32,
}

impl Thumb {
    pub(crate) fn locate(view: f32, max_offset: f32, scrolled: f32) -> Option<Self> {
        if max_offset <= 0.0 || view <= 2.0 * MARGIN {
            return None;
        }
        let track = view - 2.0 * MARGIN;
        let length = (track * view / (view + max_offset)).clamp(MIN_LENGTH.min(track), track);
        let progress = (scrolled / max_offset).clamp(0.0, 1.0);
        Some(Self {
            start: MARGIN + (track - length) * progress,
            length,
        })
    }

    pub(crate) fn scrolled_after(view: f32, max_offset: f32, scrolled: f32, delta: f32) -> f32 {
        let Some(thumb) = Self::locate(view, max_offset, scrolled) else {
            return scrolled;
        };
        let travel = view - 2.0 * MARGIN - thumb.length;
        if travel <= 0.0 {
            return scrolled;
        }
        (scrolled + delta / travel * max_offset).clamp(0.0, max_offset)
    }
}

#[cfg(test)]
#[path = "thumb_tests.rs"]
mod tests;
