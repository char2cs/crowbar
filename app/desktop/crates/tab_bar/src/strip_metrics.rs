use crate::span::Span;

pub(crate) const ROW_HEIGHT: f32 = 42.0;
pub(crate) const TAB_HEIGHT: f32 = 32.0;
pub(crate) const SLOT_MIN: f32 = 112.0;
pub(crate) const SLOT_MAX: f32 = 240.0;
pub(crate) const SHELL_MARGIN: f32 = 2.0;
pub(crate) const ROW_PADDING: f32 = 8.0;
pub(crate) const ROW_GAP: f32 = 6.0;
pub(crate) const TOGGLE_SIZE: f32 = 28.0;
pub(crate) const SCROLLER_PADDING: f32 = 4.0;
pub(crate) const EDGE_BAND: f32 = 36.0;
pub(crate) const EDGE_SPEED: f32 = 14.0;

pub(crate) fn slot_width(count: usize, available: f32) -> f32 {
    if count == 0 {
        return SLOT_MAX;
    }
    (available / as_float(count)).clamp(SLOT_MIN, SLOT_MAX)
}

pub(crate) fn slots(count: usize, available: f32) -> Vec<Span> {
    let width = slot_width(count, available);
    (0..count)
        .map(|index| Span {
            x: as_float(index) * width,
            width,
        })
        .collect()
}

pub(crate) fn as_float(count: usize) -> f32 {
    f32::from(u16::try_from(count).unwrap_or(u16::MAX))
}

pub(crate) fn edge_scroll_step(x: f32, strip: Span) -> f32 {
    let from_left = x - strip.x;
    let from_right = strip.end() - x;
    if from_left < EDGE_BAND {
        return -ramp(from_left);
    }
    if from_right < EDGE_BAND {
        return ramp(from_right);
    }
    0.0
}

fn ramp(distance: f32) -> f32 {
    let t = (1.0 - distance / EDGE_BAND).clamp(0.0, 1.0);
    (t * EDGE_SPEED).round().max(1.0)
}

#[cfg(test)]
#[path = "strip_metrics_tests.rs"]
mod tests;
