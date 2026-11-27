use std::rc::Rc;
use std::time::Instant;

use gpui::Context;

use crate::display::flicker_spinner::spinner_clock;
use crate::display::flicker_spinner::sprite::Sprite;
use crate::display::flicker_spinner::sprites;

pub(crate) struct Playback {
    sprite: Option<Rc<Sprite>>,
    started: Instant,
}

impl Playback {
    pub(crate) fn start(cx: &mut Context<'_, Self>) -> Self {
        Self {
            sprite: sprites::pick().map(Rc::new),
            started: spinner_clock::aligned_now(cx),
        }
    }

    pub(crate) fn frame(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> Option<(Rc<Sprite>, usize)> {
        let sprite = Rc::clone(self.sprite.as_ref()?);
        let elapsed = cx.background_executor().now() - self.started;
        let (frame, _) = sprite.frame_at(elapsed);
        spinner_clock::wake(cx.entity_id(), cx);
        Some((sprite, frame))
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

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
