use std::mem::discriminant;
use std::time::Duration;

use crate::halt_reason::HaltReason;
use crate::migration_step::MigrationStep;
use crate::startup_input::StartupInput;
use crate::startup_screen::StartupScreen;
use crate::startup_timing::StartupTiming;

/// Turns the stream of supervisor inputs into the screen to show.
///
/// Time is a `Duration` since any fixed origin that the caller passes with every call; the
/// machine never reads a clock and never waits. Rules:
///
/// - Progress never goes backwards within one daemon run, and a stale step is ignored.
/// - Nothing is drawn for the first `reveal_delay`, so a start that finishes at once shows no view.
/// - Once drawn, a normal screen stays `min_visible` before a different normal screen replaces it;
///   only the latest queued screen survives. Failures and updates of the same screen apply at once.
#[derive(Debug, Clone)]
pub struct StartupMachine {
    timing: StartupTiming,
    shown: StartupScreen,
    shown_at: Duration,
    revealed_at: Duration,
    queued: Option<StartupScreen>,
    step: Option<MigrationStep>,
    peak: Option<f32>,
    now: Duration,
}

impl StartupMachine {
    /// A machine showing [`StartupScreen::Starting`] from `now`.
    pub fn new(timing: StartupTiming, now: Duration) -> Self {
        Self {
            timing,
            shown: StartupScreen::Starting,
            shown_at: now,
            revealed_at: now + timing.reveal_delay(),
            queued: None,
            step: None,
            peak: None,
            now,
        }
    }

    /// Feeds one supervisor input observed at `now`.
    pub fn apply(&mut self, input: StartupInput, now: Duration) {
        self.now = self.now.max(now);
        let target = self.screen_for(input);
        self.admit(target);
        self.settle();
    }

    /// Lets time pass: shows a queued screen once it is due.
    pub fn tick(&mut self, now: Duration) {
        self.now = self.now.max(now);
        self.settle();
    }

    /// The screen to draw.
    pub fn screen(&self) -> &StartupScreen {
        &self.shown
    }

    /// Whether the screen is drawn yet; false during the initial blank time.
    pub fn is_revealed(&self) -> bool {
        self.now >= self.revealed_at
    }

    /// The next time `tick` changes what is shown or drawn, if anything is pending.
    pub fn deadline(&self) -> Option<Duration> {
        let reveal = (self.shown.is_waiting() && !self.is_revealed()).then_some(self.revealed_at);
        let release = self.queued.as_ref().map(|_| self.release_at());
        [reveal, release].into_iter().flatten().min()
    }

    fn screen_for(&mut self, input: StartupInput) -> StartupScreen {
        match input {
            StartupInput::Starting => StartupScreen::Starting,
            StartupInput::Ready => StartupScreen::Ready,
            StartupInput::Migrating { step } => self.migrating(step),
            StartupInput::Halted { reason, message } => {
                self.forget_run();
                StartupScreen::Halted {
                    reason: HaltReason::from_code(reason.as_deref()),
                    message,
                }
            },
            StartupInput::Unavailable(outage) => {
                self.forget_run();
                StartupScreen::Unavailable(outage)
            },
        }
    }

    fn migrating(&mut self, reported: Option<MigrationStep>) -> StartupScreen {
        if let Some(step) = reported.filter(|step| !self.is_stale(step)) {
            self.peak = higher(self.peak, step.fraction());
            self.step = Some(step);
        }
        StartupScreen::Migrating {
            step: self.step.clone(),
            fraction: self.peak,
        }
    }

    fn is_stale(&self, reported: &MigrationStep) -> bool {
        self.step
            .as_ref()
            .is_some_and(|current| reported.index() < current.index())
    }

    fn forget_run(&mut self) {
        self.step = None;
        self.peak = None;
    }

    fn admit(&mut self, target: StartupScreen) {
        let replaces_in_place = discriminant(&target) == discriminant(&self.shown);
        let immediate = target.is_failure() || self.shown.is_failure() || replaces_in_place;
        if immediate || !self.is_revealed() {
            self.commit(target);
            return;
        }
        self.queued = Some(target);
    }

    fn settle(&mut self) {
        if self.queued.is_none() || self.now < self.release_at() {
            return;
        }
        if let Some(screen) = self.queued.take() {
            self.commit(screen);
        }
    }

    fn commit(&mut self, screen: StartupScreen) {
        if screen.is_failure() {
            self.revealed_at = self.revealed_at.min(self.now);
        }
        self.shown = screen;
        self.shown_at = self.now;
        self.queued = None;
    }

    fn release_at(&self) -> Duration {
        self.shown_at.max(self.revealed_at) + self.timing.min_visible()
    }
}

fn higher(peak: Option<f32>, reported: Option<f32>) -> Option<f32> {
    match (peak, reported) {
        (Some(peak), Some(reported)) => Some(peak.max(reported)),
        (peak, reported) => peak.or(reported),
    }
}

#[cfg(test)]
#[path = "startup_machine_tests.rs"]
mod tests;
