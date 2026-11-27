use std::time::{Duration, Instant};

use layout::TabId;
use ui::{CubicBezier, Glide, Motion};

const SPAN: Duration = Duration::from_millis(150);

#[derive(Debug)]
pub(crate) struct Settle {
    pub(crate) tab: TabId,
    from: f32,
    glide: Glide,
}

impl Settle {
    pub(crate) fn new(
        tab: TabId,
        from: f32,
        now: Instant,
        motion: Motion,
    ) -> Self {
        let mut glide = Glide::resting(1.0, SPAN, CubicBezier::EASE_OUT);
        glide.snap(1.0);
        glide.go_to(0.0, now, motion);
        Self { tab, from, glide }
    }

    pub(crate) fn offset(
        &self,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        self.from * self.glide.value(now, motion)
    }

    pub(crate) fn is_running(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.glide.is_running(now, motion)
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use layout::TabId;
    use ui::Motion;

    use super::*;

    #[test]
    fn the_dropped_tab_starts_at_the_ghost_offset_and_ends_in_its_slot() {
        let start = Instant::now();
        let settle = Settle::new(TabId(3), 80.0, start, Motion::default());
        assert_eq!(
            settle.offset(start, Motion::default()),
            80.0,
            "starts at the ghost"
        );
        assert!(settle.is_running(start, Motion::default()), "gliding");
        let end = start + Duration::from_millis(150);
        assert_eq!(settle.offset(end, Motion::default()), 0.0, "in its slot");
        assert!(!settle.is_running(end, Motion::default()), "done");
    }
}
