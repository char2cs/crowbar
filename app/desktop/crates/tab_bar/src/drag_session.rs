use std::time::Instant;

use gpui::{Pixels, Point};
use ui::{CubicBezier, Glide, Motion, timing};

use crate::strip_drag::StripDrag;

#[derive(Debug)]
pub(crate) struct DragSession {
    pub(crate) engine: StripDrag,
    pub(crate) pointer: Point<Pixels>,
    pub(crate) grab_y: f32,
    slides: Vec<Glide>,
}

impl DragSession {
    pub(crate) fn new(engine: StripDrag, pointer: Point<Pixels>, grab_y: f32, tabs: usize) -> Self {
        let slide = Glide::resting(0.0, timing::DEFAULT, CubicBezier::EASE_OUT);
        Self {
            engine,
            pointer,
            grab_y,
            slides: vec![slide; tabs],
        }
    }

    pub(crate) fn follow(&mut self, pointer: Point<Pixels>, now: Instant, motion: Motion) {
        self.pointer = pointer;
        for (index, slide) in self.slides.iter_mut().enumerate() {
            slide.go_to(self.engine.offset(index), now, motion);
        }
    }

    pub(crate) fn slide(&self, index: usize, now: Instant, motion: Motion) -> f32 {
        self.slides
            .get(index)
            .map_or(0.0, |slide| slide.value(now, motion))
    }

    pub(crate) fn is_sliding(&self, now: Instant, motion: Motion) -> bool {
        self.slides
            .iter()
            .any(|slide| slide.is_running(now, motion))
    }
}

#[cfg(test)]
#[path = "drag_session_tests.rs"]
mod tests;
