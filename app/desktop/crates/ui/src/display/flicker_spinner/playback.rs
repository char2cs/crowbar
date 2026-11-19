//! Where one spinner is in its sprite, and the one timer that repaints it at its next step.

use std::rc::Rc;
use std::time::{Duration, Instant};

use gpui::{Context, Task};

use crate::display::flicker_spinner::sprite::Sprite;
use crate::display::flicker_spinner::sprites;

pub(crate) struct Playback {
    sprite: Option<Rc<Sprite>>,
    started: Instant,
    step: Option<Task<()>>,
}

impl Playback {
    pub(crate) fn start(cx: &mut Context<'_, Self>) -> Self {
        Self {
            sprite: sprites::pick().map(Rc::new),
            started: cx.background_executor().now(),
            step: None,
        }
    }

    pub(crate) fn frame(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> Option<(Rc<Sprite>, usize)> {
        let sprite = Rc::clone(self.sprite.as_ref()?);
        let elapsed = cx.background_executor().now() - self.started;
        let (frame, left) = sprite.frame_at(elapsed);
        self.arm(left, cx);
        Some((sprite, frame))
    }

    fn arm(
        &mut self,
        left: Duration,
        cx: &mut Context<'_, Self>,
    ) {
        if self.step.is_some() {
            return;
        }
        self.step = Some(cx.spawn(async move |playback, cx| {
            cx.background_executor().timer(left).await;
            playback.update(cx, step).unwrap_or_default();
        }));
    }
}

fn step(
    playback: &mut Playback,
    cx: &mut Context<'_, Playback>,
) {
    playback.step = None;
    cx.notify();
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    const JUST_BEFORE: Duration = Duration::from_millis(1);

    fn frame_of(
        playback: &gpui::Entity<Playback>,
        cx: &mut TestAppContext,
    ) -> usize {
        playback.update(cx, |playback, cx| {
            playback.frame(cx).map_or(0, |(_, frame)| frame)
        })
    }

    #[gpui::test]
    fn the_frame_advances_when_its_step_timer_fires(cx: &mut TestAppContext) {
        let playback = cx.new(Playback::start);
        let step = playback.update(cx, |playback, cx| {
            playback
                .frame(cx)
                .map(|(sprite, _)| sprite.frame_at(Duration::ZERO).1)
        });
        let step = step.expect("a sprite");
        assert_eq!(frame_of(&playback, cx), 0, "the first frame");
        cx.executor()
            .advance_clock(step.saturating_sub(JUST_BEFORE));
        assert_eq!(frame_of(&playback, cx), 0, "held until the step");
        cx.executor().advance_clock(JUST_BEFORE);
        assert_eq!(frame_of(&playback, cx), 1, "the next frame at the step");
    }
}
