//! Hand-over of the startup view: the wake timer, the fade and the final event.

use std::time::Duration;

use gpui::{Context, Task};
use ui::{Motion, timing};

use super::{StartupView, note_dropped};
use crate::startup_machine::StartupScreen;
use crate::startup_view::handover::Handover;
use crate::startup_view::startup_event::StartupEvent;

impl StartupView {
    pub(crate) fn remember_drawn_fraction(&mut self) {
        if let StartupScreen::Migrating {
            fraction: Some(fraction),
            ..
        } = self.machine.screen()
        {
            self.drawn_fraction = *fraction;
        }
    }

    pub(crate) fn sync(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.was_revealed |= self.machine.is_revealed();
        if matches!(self.machine.screen(), StartupScreen::Ready) {
            self.begin_handover(cx);
        }
        self.schedule_wake(cx);
        cx.notify();
    }

    pub(crate) fn begin_handover(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.handover != Handover::Waiting {
            return;
        }
        if !self.was_revealed {
            self.finish(cx);
            return;
        }
        self.handover = Handover::Fading;
        let fade = Motion::of(cx).duration(timing::SLOW);
        self.wake = Some(Self::after(fade, cx, Self::finish));
    }

    pub(crate) fn finish(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.handover = Handover::Done;
        self.wake = None;
        cx.emit(StartupEvent::HandedOver);
        cx.notify();
    }

    pub(crate) fn schedule_wake(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.handover != Handover::Waiting {
            return;
        }
        let now = self.clock.now();
        self.wake = self
            .machine
            .deadline()
            .map(|deadline| Self::after(deadline.saturating_sub(now), cx, Self::tick));
    }

    pub(crate) fn after(
        delay: Duration,
        cx: &mut Context<'_, Self>,
        then: fn(&mut Self, &mut Context<'_, Self>),
    ) -> Task<()> {
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(delay).await;
            note_dropped(this.update(cx, then));
        })
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::startup_machine::AppPhase;

    use crate::startup_machine::Outage;
    use crate::startup_machine::StartupInput;
    use crate::startup_machine::StartupTiming;
    use crate::startup_view::tests::{REVEAL, VISIBLE, at, feed, instant_timing, open, pass};

    #[gpui::test]
    fn a_start_that_finishes_inside_the_blank_time_hands_over_without_a_fade(
        cx: &mut TestAppContext
    ) {
        let (harness, cx) = open(cx, StartupTiming::new(REVEAL, VISIBLE));
        harness.clock.set(at(100));
        feed(&harness, cx, StartupInput::Ready);
        assert!(harness.view.read_with(cx, |view, _| view.is_handed_over()));
        assert_eq!(*harness.events.borrow(), [StartupEvent::HandedOver]);
    }

    #[gpui::test]
    fn a_visible_start_waits_out_the_minimum_then_fades_and_hands_over(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, StartupTiming::new(REVEAL, VISIBLE));
        pass(&harness, cx, at(300));
        feed(&harness, cx, StartupInput::Ready);
        assert_eq!(
            harness.view.read_with(cx, |view, _| view.phase()),
            AppPhase::Starting
        );
        pass(&harness, cx, REVEAL + VISIBLE);
        assert_eq!(
            harness.view.read_with(cx, |view, _| view.phase()),
            AppPhase::Ready
        );
        assert!(!harness.view.read_with(cx, |view, _| view.is_handed_over()));
        pass(&harness, cx, REVEAL + VISIBLE + timing::SLOW);
        assert!(harness.view.read_with(cx, |view, _| view.is_handed_over()));
        assert_eq!(*harness.events.borrow(), [StartupEvent::HandedOver]);
    }

    #[gpui::test]
    fn inputs_after_the_hand_over_are_ignored(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, instant_timing());
        feed(&harness, cx, StartupInput::Ready);
        pass(&harness, cx, timing::SLOW);
        feed(&harness, cx, StartupInput::Unavailable(Outage::Stopped));
        assert_eq!(
            harness.view.read_with(cx, |view, _| view.phase()),
            AppPhase::Ready
        );
        assert_eq!(harness.events.borrow().len(), 1);
    }
}
