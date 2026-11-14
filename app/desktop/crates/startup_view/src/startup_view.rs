use std::rc::Rc;
use std::time::Duration;

use gpui::{
    AnimationExt, Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Task, Window,
    div, px,
};
use protocol::AppPhase;
use ui::{CubicBezier, Motion, Surface, Theme, font, timing};

use crate::halt_panel::HaltPanel;
use crate::handover::Handover;
use crate::outage_panel::OutagePanel;
use crate::startup_actions::StartupActions;
use crate::startup_clock::StartupClock;
use crate::startup_event::StartupEvent;
use crate::startup_input::StartupInput;
use crate::startup_machine::StartupMachine;
use crate::startup_screen::StartupScreen;
use crate::startup_timing::StartupTiming;
use crate::waiting_panel::WaitingPanel;
use crate::wordmark::Wordmark;

/// The full-screen startup and data-migration view.
///
/// It covers the whole window, hides the layout, follows the daemon through
/// [`StartupInput`]s, and when the daemon serves it fades out and emits
/// [`StartupEvent::HandedOver`]. Inputs after the hand-over are ignored: a later outage is the
/// normal layout's business.
pub struct StartupView {
    machine: StartupMachine,
    clock: Rc<dyn StartupClock>,
    actions: Rc<dyn StartupActions>,
    drawn_fraction: f32,
    was_revealed: bool,
    handover: Handover,
    wake: Option<Task<()>>,
    titled: String,
}

impl StartupView {
    /// A view in its starting state; `actions` receives the user's commands.
    pub fn new(
        timing: StartupTiming,
        clock: Rc<dyn StartupClock>,
        actions: Rc<dyn StartupActions>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let mut view = Self {
            machine: StartupMachine::new(timing, clock.now()),
            clock,
            actions,
            drawn_fraction: 0.0,
            was_revealed: false,
            handover: Handover::Waiting,
            wake: None,
            titled: String::new(),
        };
        view.schedule_wake(cx);
        view
    }

    /// Feeds what the daemon supervisor reported.
    pub fn apply(&mut self, input: StartupInput, cx: &mut Context<'_, Self>) {
        if self.handover != Handover::Waiting {
            return;
        }
        self.remember_drawn_fraction();
        self.machine.apply(input, self.clock.now());
        self.sync(cx);
    }

    /// The protocol phase of what is shown; only [`AppPhase::Ready`] lets the layout through.
    pub fn phase(&self) -> AppPhase {
        self.machine.screen().phase()
    }

    /// The text a screen reader should announce for what is shown.
    pub fn announcement(&self) -> String {
        self.machine.screen().announcement()
    }

    /// Whether the view has handed over and can be removed.
    pub fn is_handed_over(&self) -> bool {
        self.handover == Handover::Done
    }

    fn tick(&mut self, cx: &mut Context<'_, Self>) {
        self.machine.tick(self.clock.now());
        self.sync(cx);
    }

    fn remember_drawn_fraction(&mut self) {
        if let StartupScreen::Migrating {
            fraction: Some(fraction),
            ..
        } = self.machine.screen()
        {
            self.drawn_fraction = *fraction;
        }
    }

    fn sync(&mut self, cx: &mut Context<'_, Self>) {
        self.was_revealed |= self.machine.is_revealed();
        if matches!(self.machine.screen(), StartupScreen::Ready) {
            self.begin_handover(cx);
        }
        self.schedule_wake(cx);
        cx.notify();
    }

    fn begin_handover(&mut self, cx: &mut Context<'_, Self>) {
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

    fn finish(&mut self, cx: &mut Context<'_, Self>) {
        self.handover = Handover::Done;
        self.wake = None;
        cx.emit(StartupEvent::HandedOver);
        cx.notify();
    }

    fn schedule_wake(&mut self, cx: &mut Context<'_, Self>) {
        if self.handover != Handover::Waiting {
            return;
        }
        let now = self.clock.now();
        self.wake = self
            .machine
            .deadline()
            .map(|deadline| Self::after(deadline.saturating_sub(now), cx, Self::tick));
    }

    fn after(
        delay: Duration,
        cx: &mut Context<'_, Self>,
        then: fn(&mut Self, &mut Context<'_, Self>),
    ) -> Task<()> {
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(delay).await;
            this.update(cx, then).ok();
        })
    }

    fn stage(&self) -> gpui::Div {
        div()
            .flex()
            .flex_col()
            .items_center()
            .gap(px(36.0))
            .child(Wordmark)
            .child(self.content())
    }

    fn content(&self) -> gpui::AnyElement {
        let actions = &self.actions;
        match self.machine.screen() {
            StartupScreen::Starting | StartupScreen::Ready => {
                WaitingPanel::starting().into_any_element()
            },
            StartupScreen::Migrating { step, fraction } => {
                WaitingPanel::migrating(step.as_ref(), *fraction, self.drawn_fraction)
                    .into_any_element()
            },
            StartupScreen::Halted { reason, message } => {
                HaltPanel::new(reason, message.clone(), actions).into_any_element()
            },
            StartupScreen::Unavailable(outage) => {
                OutagePanel::new(outage, actions).into_any_element()
            },
        }
    }
}

impl std::fmt::Debug for StartupView {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("StartupView")
            .field("screen", self.machine.screen())
            .field("handover", &self.handover)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<StartupEvent> for StartupView {}

impl Render for StartupView {
    fn render(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let motion = Motion::of(cx);
        let title = format!("Crowbar - {}", self.machine.screen().headline());
        if self.titled != title {
            window.set_window_title(&title);
            self.titled = title;
        }
        let backdrop = div()
            .size_full()
            .flex()
            .items_center()
            .justify_center()
            .bg(palette.background())
            .text_color(palette.foreground())
            .font_family(font::UI_FAMILY);
        match self.handover {
            Handover::Done => div().into_any_element(),
            Handover::Fading => backdrop
                .child(self.stage())
                .with_animation(
                    "startup-fade-out",
                    motion.animation(timing::SLOW, CubicBezier::DEFAULT),
                    |backdrop, delta| backdrop.opacity(1.0 - delta),
                )
                .into_any_element(),
            Handover::Waiting if self.machine.is_revealed() => backdrop
                .child(self.stage().with_animation(
                    "startup-fade-in",
                    motion.animation(timing::SLOW, CubicBezier::EASE_OUT),
                    Styled::opacity,
                ))
                .into_any_element(),
            Handover::Waiting => backdrop.into_any_element(),
        }
    }
}

#[cfg(test)]
#[path = "startup_view_tests.rs"]
mod tests;
