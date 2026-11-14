use std::time::Instant;

use ui::{CubicBezier, Glide, Motion};

use crate::tab_drop::TabDrop;

const LINE_GLIDE: std::time::Duration = std::time::Duration::from_millis(60);

#[derive(Debug)]
pub(crate) struct IncomingDrop {
    pub(crate) drop: TabDrop,
    line: Glide,
}

impl IncomingDrop {
    pub(crate) fn new(drop: TabDrop, line_x: f32) -> Self {
        Self {
            drop,
            line: Glide::resting(line_x, LINE_GLIDE, CubicBezier::EASE_OUT),
        }
    }

    pub(crate) fn move_to(&mut self, drop: TabDrop, line_x: f32, (now, motion): (Instant, Motion)) {
        self.drop = drop;
        self.line.go_to(line_x, now, motion);
    }

    pub(crate) fn line_x(&self, now: Instant, motion: Motion) -> f32 {
        self.line.value(now, motion)
    }

    pub(crate) fn is_moving(&self, now: Instant, motion: Motion) -> bool {
        self.line.is_running(now, motion)
    }
}

#[cfg(test)]
#[path = "incoming_drop_tests.rs"]
mod tests;
