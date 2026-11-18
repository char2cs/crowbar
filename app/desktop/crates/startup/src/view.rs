use std::rc::Rc;

use crate::machine::AppPhase;
use gpui::{
    AnimationExt, App, Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Task,
    Window, div, px,
};
use ui::{CubicBezier, Motion, Surface, Theme, timing};

use crate::actions::Actions;
use crate::machine::Input;
use crate::machine::Machine;
use crate::machine::Screen;
use crate::machine::Timing;
use crate::screens::HaltPanel;
use crate::screens::OutagePanel;
use crate::screens::WaitingPanel;
use crate::screens::wordmark;
use crate::view::handover::Handover;

const STAGE_GAP: f32 = 36.0;

/// The full-screen startup and data-migration view.
///
/// It covers the whole window, hides the layout, follows the daemon through
/// [`Input`]s, and when the daemon serves it fades out and emits
/// [`Event::HandedOver`]. Inputs after the hand-over are ignored: a later outage is the
/// normal layout's business.
pub struct View {
    machine: Machine,
    clock: Rc<dyn Clock>,
    actions: Rc<dyn Actions>,
    drawn_fraction: f32,
    was_revealed: bool,
    handover: Handover,
    wake: Option<Task<()>>,
    titled: String,
}

mod adapters;
mod clock;
mod event;
mod handing_over;
mod handover;

pub use adapters::MonotonicClock;
pub use clock::Clock;
pub use event::Event;

impl View {
    /// A view in its starting state; `actions` receives the user's commands.
    pub fn new(
        timing: Timing,
        clock: Rc<dyn Clock>,
        actions: Rc<dyn Actions>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let mut view = Self {
            machine: Machine::new(timing, clock.now()),
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
        input: Input,
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
            Screen::Starting | Screen::Ready => WaitingPanel::starting().into_any_element(),
            Screen::Migrating { step, fraction } => {
                WaitingPanel::migrating(step.as_ref(), *fraction, self.drawn_fraction)
                    .into_any_element()
            },
            Screen::Halted { reason, message } => {
                HaltPanel::new(reason, message.clone(), actions).into_any_element()
            },
            Screen::Unavailable(outage) => OutagePanel::new(outage, actions).into_any_element(),
        }
    }
}

impl std::fmt::Debug for View {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("View")
            .field("screen", self.machine.screen())
            .field("handover", &self.handover)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<Event> for View {}

impl Render for View {
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
    use crate::command::Command;
    use crate::machine::MigrationStep;
    use crate::machine::Outage;
    use crate::test_support::{ManualClock, RecordingActions, click_across};

    pub(super) const REVEAL: Duration = Duration::from_millis(250);
    pub(super) const VISIBLE: Duration = Duration::from_millis(600);

    pub(super) struct Harness {
        pub(super) view: Entity<View>,
        pub(super) clock: Rc<ManualClock>,
        pub(super) actions: Rc<RecordingActions>,
        pub(super) events: Rc<RefCell<Vec<Event>>>,
    }

    pub(super) fn at(millis: u64) -> Duration {
        Duration::from_millis(millis)
    }

    pub(super) fn open(
        cx: &mut TestAppContext,
        timing: Timing,
    ) -> (Harness, &mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).expect("the design system installs"));
        let clock = Rc::new(ManualClock::default());
        let actions = Rc::new(RecordingActions::default());
        let (view_clock, view_actions) = (Rc::clone(&clock), Rc::clone(&actions));
        let (view, cx) =
            cx.add_window_view(move |_, cx| View::new(timing, view_clock, view_actions, cx));
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
        view: &Entity<View>,
        sink: Rc<RefCell<Vec<Event>>>,
        cx: &mut App,
    ) {
        cx.subscribe(view, move |_, event: &Event, _| {
            sink.borrow_mut().push(*event);
        })
        .detach();
    }

    pub(super) fn feed(
        harness: &Harness,
        cx: &mut VisualTestContext,
        input: Input,
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

    pub(super) fn instant_timing() -> Timing {
        Timing::new(Duration::ZERO, Duration::ZERO)
    }

    #[gpui::test]
    fn migration_progress_reaches_the_protocol_phase_and_the_announcement(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, instant_timing());
        let step = MigrationStep::new("Rebuild index", 2, 5);
        feed(&harness, cx, Input::Migrating { step: Some(step) });
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
            Input::Halted {
                reason: Some("layout_newer".to_owned()),
                message: "layout 9 is newer than 7".to_owned(),
            },
        );
        click_everywhere(cx);
        let performed = harness.actions.performed();
        assert!(performed.contains(&Command::Quit), "{performed:?}");
        assert!(!performed.contains(&Command::Retry), "{performed:?}");
    }

    #[gpui::test]
    fn a_crash_loop_can_retry(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, instant_timing());
        feed(
            &harness,
            cx,
            Input::Unavailable(Outage::GaveUp {
                report: "log".to_owned(),
            }),
        );
        click_everywhere(cx);
        let performed = harness.actions.performed();
        assert!(performed.contains(&Command::Retry), "{performed:?}");
    }
}
