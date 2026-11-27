use std::time::{Duration, Instant};

use gpui::{App, Context, Task};

use crate::control::editor::field::Blinking;
use crate::foundation::{CaretBlink, Motion, Pose};

#[derive(Default)]
pub(crate) struct Blink {
    hidden: bool,
    edge: Option<Task<()>>,
    due: Option<Instant>,
}

impl Blink {
    pub(crate) fn is_shown(&self) -> bool {
        !self.hidden
    }

    pub(crate) fn restart(&mut self) {
        self.hidden = false;
        self.edge = None;
        self.due = None;
    }

    fn flip(&mut self) {
        self.hidden = !self.hidden;
        self.edge = None;
    }
}

pub(crate) fn blinks(
    caret: Pose,
    cx: &App,
) -> Pose {
    let still = Motion::of(cx).is_reduced() || CaretBlink::of(cx) == CaretBlink::Steady;
    Pose::from(caret == Pose::On && !still)
}

pub(crate) fn tend<T: Blinking>(
    field: &mut T,
    lit: Pose,
    cx: &mut Context<'_, T>,
) {
    let phase = CaretBlink::of(cx).phase().filter(|_| lit == Pose::On);
    let Some(phase) = phase else {
        field.blink_mut().restart();
        return;
    };
    if field.blink().edge.is_some() {
        return;
    }
    let now = cx.background_executor().now();
    let due = next_due(field.blink().due, now, phase);
    let edge = cx.spawn(async move |field, cx| {
        cx.background_executor().timer(due - now).await;
        field.update(cx, pass_edge).unwrap_or_default();
    });
    let blink = field.blink_mut();
    blink.edge = Some(edge);
    blink.due = Some(due);
}

fn next_due(
    last: Option<Instant>,
    now: Instant,
    phase: Duration,
) -> Instant {
    last.map(|edge| edge + phase)
        .filter(|next| *next > now)
        .unwrap_or(now + phase)
}

fn pass_edge<T: Blinking>(
    field: &mut T,
    cx: &mut Context<'_, T>,
) {
    field.blink_mut().flip();
    cx.notify();
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_restart_shows_the_caret_and_cancels_the_pending_edge() {
        let mut blink = Blink::default();
        blink.flip();
        assert!(!blink.is_shown(), "a flip hides a shown caret");
        blink.restart();
        assert!(blink.is_shown(), "a restart shows it again");
        assert!(blink.edge.is_none(), "and drops the timer");
    }

    #[test]
    fn a_late_frame_keeps_the_next_edge_on_its_own_time() {
        let edge = Instant::now();
        let phase = Duration::from_millis(600);
        let late = Duration::from_millis(100);
        let next = next_due(Some(edge), edge + late, phase);
        assert_eq!(
            next,
            edge + phase,
            "one phase after the edge, not the frame"
        );
        let missed = next_due(Some(edge), edge + phase * 2, phase);
        assert_eq!(
            missed,
            edge + phase * 3,
            "a missed edge restarts from the frame"
        );
        assert_eq!(next_due(None, edge, phase), edge + phase, "the first edge");
    }
}
