use gpui::{Pixels, Point};

const THRESHOLD: f32 = 8.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Press {
    pub(crate) index: usize,
    pub(crate) origin: Point<Pixels>,
}

impl Press {
    pub(crate) fn has_moved_past_threshold(&self, position: Point<Pixels>) -> bool {
        let dx = f32::from(position.x - self.origin.x);
        let dy = f32::from(position.y - self.origin.y);
        dx.hypot(dy) > THRESHOLD
    }
}

#[cfg(test)]
#[path = "press_tests.rs"]
mod tests;
