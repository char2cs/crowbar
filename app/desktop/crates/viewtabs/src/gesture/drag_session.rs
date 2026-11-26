use std::time::Instant;

use gpui::{Pixels, Point};
use ui::{CubicBezier, Glide, Motion, timing};

use crate::gesture::strip_drag::StripDrag;

const SOURCE_IN_STRIP: f32 = 0.0;
const SOURCE_OFF_STRIP: f32 = 0.4;

#[derive(Debug)]
pub(crate) struct DragSession {
    pub(crate) engine: StripDrag,
    pub(crate) pointer: Point<Pixels>,
    pub(crate) grab_y: f32,
    pub(crate) out: bool,
    slides: Vec<Glide>,
}

impl DragSession {
    pub(crate) fn new(
        engine: StripDrag,
        pointer: Point<Pixels>,
        grab_y: f32,
        tabs: usize,
    ) -> Self {
        let slide = Glide::resting(0.0, timing::DEFAULT, CubicBezier::EASE_OUT);
        Self {
            engine,
            pointer,
            grab_y,
            out: false,
            slides: vec![slide; tabs],
        }
    }

    pub(crate) fn follow(
        &mut self,
        pointer: Point<Pixels>,
        now: Instant,
        motion: Motion,
    ) {
        self.pointer = pointer;
        for (index, slide) in self.slides.iter_mut().enumerate() {
            slide.go_to(self.engine.offset(index), now, motion);
        }
    }

    pub(crate) fn slide(
        &self,
        index: usize,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        self.slides
            .get(index)
            .map_or(0.0, |slide| slide.value(now, motion))
    }

    pub(crate) fn aim(
        &mut self,
        x: f32,
    ) {
        if self.out {
            self.engine.home();
            return;
        }
        self.engine.moved(x);
    }

    pub(crate) fn source_opacity(&self) -> f32 {
        if self.out {
            return SOURCE_OFF_STRIP;
        }
        SOURCE_IN_STRIP
    }

    pub(crate) fn is_sliding(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.slides
            .iter()
            .any(|slide| slide.is_running(now, motion))
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use gpui::{point, px};
    use layout::TabId;
    use ui::Motion;

    use super::*;
    use crate::span::Span;

    fn session() -> DragSession {
        let tabs: Vec<TabId> = (0..3).map(TabId).collect();
        let spans: Vec<Span> = (0..3u16)
            .map(|i| Span {
                x: f32::from(i) * 100.0,
                width: 100.0,
            })
            .collect();
        let engine = StripDrag::begin(tabs, spans, 0, 50.0).expect("engine");
        DragSession::new(engine, point(px(50.0), px(20.0)), 16.0, 3)
    }

    #[test]
    fn neighbours_slide_over_150_ms_when_the_target_moves() {
        let start = Instant::now();
        let mut session = session();
        session.engine.moved(160.0);
        session.follow(point(px(160.0), px(20.0)), start, Motion::default());
        assert_eq!(
            session.slide(1, start, Motion::default()),
            0.0,
            "starts in place"
        );
        assert!(session.is_sliding(start, Motion::default()), "sliding");
        let settled = start + Duration::from_millis(150);
        assert_eq!(
            session.slide(1, settled, Motion::default()),
            -100.0,
            "one pitch to the left"
        );
        assert_eq!(
            session.slide(0, settled, Motion::default()),
            0.0,
            "the dragged tab stays at zero"
        );
        assert!(!session.is_sliding(settled, Motion::default()), "done");
    }
}
