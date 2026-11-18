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
    pub(crate) fn new(
        drop: TabDrop,
        line_x: f32,
    ) -> Self {
        Self {
            drop,
            line: Glide::resting(line_x, LINE_GLIDE, CubicBezier::EASE_OUT),
        }
    }

    pub(crate) fn move_to(
        &mut self,
        drop: TabDrop,
        line_x: f32,
        (now, motion): (Instant, Motion),
    ) {
        self.drop = drop;
        self.line.go_to(line_x, now, motion);
    }

    pub(crate) fn line_x(
        &self,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        self.line.value(now, motion)
    }

    pub(crate) fn is_moving(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.line.is_running(now, motion)
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use layout::TabId;
    use ui::Motion;

    use super::*;

    #[test]
    fn the_drop_line_glides_to_its_new_position_in_sixty_milliseconds() {
        let start = Instant::now();
        let mut incoming = IncomingDrop::new(TabDrop::Reorder(0), 0.0);
        incoming.move_to(TabDrop::Merge(TabId(1)), 100.0, (start, Motion::default()));
        assert_eq!(incoming.drop, TabDrop::Merge(TabId(1)), "new target");
        assert_eq!(
            incoming.line_x(start, Motion::default()),
            0.0,
            "starts where it was"
        );
        assert!(incoming.is_moving(start, Motion::default()), "moving");
        let end = start + Duration::from_millis(60);
        assert_eq!(incoming.line_x(end, Motion::default()), 100.0, "arrived");
        assert!(!incoming.is_moving(end, Motion::default()), "stopped");
    }
}
