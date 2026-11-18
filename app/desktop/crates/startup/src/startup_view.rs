use std::rc::Rc;

use crate::startup_machine::AppPhase;
use gpui::{
    AnimationExt, App, Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Task,
    Window, div, px,
};
use ui::{CubicBezier, Motion, Surface, Theme, timing};

use crate::screens::HaltPanel;
use crate::screens::OutagePanel;
use crate::screens::WaitingPanel;
use crate::screens::wordmark;
use crate::startup_actions::StartupActions;
use crate::startup_machine::StartupInput;
use crate::startup_machine::StartupMachine;
use crate::startup_machine::StartupScreen;
use crate::startup_machine::StartupTiming;
use crate::startup_view::handover::Handover;

const STAGE_GAP: f32 = 36.0;

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

mod adapters;
mod handing_over;
mod handover;
mod startup_clock;
mod startup_event;

pub use adapters::MonotonicClock;
pub use startup_clock::StartupClock;
pub use startup_event::StartupEvent;

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
    pub fn apply(
        &mut self,
        input: StartupInput,
        cx: &mut Context<'_, Self>,
    ) {
        if self.handover != Handover::Waiting {
            return;
        }
        self.remember_drawn_fraction();
        self.machine.apply(input, self.clock.now());
        self.sync(cx);
    }

    /// The app phase of what is shown; only [`AppPhase::Ready`] lets the layout through.
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

    fn tick(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.machine.tick(self.clock.now());
        self.sync(cx);
    }

    fn stage(
        &self,
        cx: &App,
    ) -> gpui::Div {
        div()
            .flex()
            .flex_col()
            .items_center()
            .gap(px(STAGE_GAP))
            .child(wordmark(cx))
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
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("StartupView")
            .field("screen", self.machine.screen())
            .field("handover", &self.handover)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<StartupEvent> for StartupView {}

impl Render for StartupView {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
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
            .font_family(Theme::of(cx).ui_family());
        match self.handover {
            Handover::Done => div().into_any_element(),
            Handover::Fading => backdrop
                .child(self.stage(cx))
                .with_animation(
                    "startup-fade-out",
                    motion.animation(timing::SLOW, CubicBezier::DEFAULT),
                    |backdrop, delta| backdrop.opacity(1.0 - delta),
                )
                .into_any_element(),
            Handover::Waiting if self.machine.is_revealed() => backdrop
                .child(self.stage(cx).with_animation(
                    "startup-fade-in",
                    motion.animation(timing::SLOW, CubicBezier::EASE_OUT),
                    Styled::opacity,
                ))
                .into_any_element(),
            Handover::Waiting => backdrop.into_any_element(),
        }
    }
}

fn note_dropped<Failure: std::fmt::Display>(update: Result<(), Failure>) {
    if let Err(error) = update {
        tracing::debug!(%error, "startup view dropped before its timer fired");
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;
    use std::time::Duration;

    use gpui::{Entity, TestAppContext, VisualTestContext};

    use super::*;
    use crate::startup_command::StartupCommand;
    use crate::startup_machine::MigrationStep;
    use crate::startup_machine::Outage;
    use crate::test_support::{ManualClock, RecordingActions, click_across};

    pub(super) const REVEAL: Duration = Duration::from_millis(250);
    pub(super) const VISIBLE: Duration = Duration::from_millis(600);

    pub(super) struct Harness {
        pub(super) view: Entity<StartupView>,
        pub(super) clock: Rc<ManualClock>,
        pub(super) actions: Rc<RecordingActions>,
        pub(super) events: Rc<RefCell<Vec<StartupEvent>>>,
    }

    pub(super) fn at(millis: u64) -> Duration {
        Duration::from_millis(millis)
    }

    pub(super) fn open(
        cx: &mut TestAppContext,
        timing: StartupTiming,
    ) -> (Harness, &mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).expect("the design system installs"));
        let clock = Rc::new(ManualClock::default());
        let actions = Rc::new(RecordingActions::default());
        let (view_clock, view_actions) = (Rc::clone(&clock), Rc::clone(&actions));
        let (view, cx) =
            cx.add_window_view(move |_, cx| StartupView::new(timing, view_clock, view_actions, cx));
        let events = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&events);
        cx.update(|_, cx| record_events(&view, sink, cx));
        let harness = Harness {
            view,
            clock,
            actions,
            events,
        };
        (harness, cx)
    }

    fn record_events(
        view: &Entity<StartupView>,
        sink: Rc<RefCell<Vec<StartupEvent>>>,
        cx: &mut App,
    ) {
        cx.subscribe(view, move |_, event: &StartupEvent, _| {
            sink.borrow_mut().push(*event);
        })
        .detach();
    }

    pub(super) fn feed(
        harness: &Harness,
        cx: &mut VisualTestContext,
        input: StartupInput,
    ) {
        harness.view.update(cx, |view, cx| view.apply(input, cx));
    }

    pub(super) fn pass(
        harness: &Harness,
        cx: &mut VisualTestContext,
        to: Duration,
    ) {
        let elapsed = to.saturating_sub(harness.clock.now());
        harness.clock.set(to);
        cx.executor().advance_clock(elapsed);
        cx.run_until_parked();
    }

    pub(super) fn instant_timing() -> StartupTiming {
        StartupTiming::new(Duration::ZERO, Duration::ZERO)
    }

    #[gpui::test]
    fn migration_progress_reaches_the_protocol_phase_and_the_announcement(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, instant_timing());
        let step = MigrationStep::new("Rebuild index", 2, 5);
        feed(&harness, cx, StartupInput::Migrating { step: Some(step) });
        let (phase, spoken) = harness
            .view
            .read_with(cx, |view, _| (view.phase(), view.announcement()));
        assert_eq!(
            phase,
            AppPhase::Migrating {
                state: "Step 2 of 5: Rebuild index".to_owned(),
                progress: Some(0.4)
            }
        );
        assert_eq!(
            spoken,
            "Upgrading your data. Step 2 of 5: Rebuild index. 40 percent."
        );
    }

    fn click_everywhere(cx: &mut VisualTestContext) {
        cx.run_until_parked();
        let area = cx.update(|window, _| gpui::Bounds {
            origin: gpui::Point::default(),
            size: window.viewport_size(),
        });
        click_across(cx, area);
    }

    #[gpui::test]
    fn the_halt_screen_can_quit(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, instant_timing());
        feed(
            &harness,
            cx,
            StartupInput::Halted {
                reason: Some("layout_newer".to_owned()),
                message: "layout 9 is newer than 7".to_owned(),
            },
        );
        click_everywhere(cx);
        let performed = harness.actions.performed();
        assert!(performed.contains(&StartupCommand::Quit), "{performed:?}");
        assert!(!performed.contains(&StartupCommand::Retry), "{performed:?}");
    }

    #[gpui::test]
    fn a_crash_loop_can_retry(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, instant_timing());
        feed(
            &harness,
            cx,
            StartupInput::Unavailable(Outage::GaveUp {
                report: "log".to_owned(),
            }),
        );
        click_everywhere(cx);
        let performed = harness.actions.performed();
        assert!(performed.contains(&StartupCommand::Retry), "{performed:?}");
    }
}
