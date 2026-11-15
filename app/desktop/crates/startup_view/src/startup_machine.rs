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
    pub fn new(
        timing: StartupTiming,
        now: Duration,
    ) -> Self {
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
    pub fn apply(
        &mut self,
        input: StartupInput,
        now: Duration,
    ) {
        self.now = self.now.max(now);
        let target = self.screen_for(input);
        self.admit(target);
        self.settle();
    }

    /// Lets time pass: shows a queued screen once it is due.
    pub fn tick(
        &mut self,
        now: Duration,
    ) {
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

    fn screen_for(
        &mut self,
        input: StartupInput,
    ) -> StartupScreen {
        match input {
            StartupInput::Starting => StartupScreen::Starting,
            StartupInput::Ready => StartupScreen::Ready,
            StartupInput::Migrating { step } => self.migrating(step),
            StartupInput::Halted { reason, message } => self.halted(reason.as_deref(), message),
            StartupInput::Unavailable(outage) => {
                self.forget_run();
                StartupScreen::Unavailable(outage)
            },
        }
    }

    fn halted(
        &mut self,
        code: Option<&str>,
        message: String,
    ) -> StartupScreen {
        self.forget_run();
        StartupScreen::Halted {
            reason: HaltReason::from_code(code),
            message,
        }
    }

    fn migrating(
        &mut self,
        reported: Option<MigrationStep>,
    ) -> StartupScreen {
        if let Some(step) = reported.filter(|step| !self.is_stale(step)) {
            self.peak = higher(self.peak, step.fraction());
            self.step = Some(step);
        }
        StartupScreen::Migrating {
            step: self.step.clone(),
            fraction: self.peak,
        }
    }

    fn is_stale(
        &self,
        reported: &MigrationStep,
    ) -> bool {
        self.step
            .as_ref()
            .is_some_and(|current| reported.index() < current.index())
    }

    fn forget_run(&mut self) {
        self.step = None;
        self.peak = None;
    }

    fn admit(
        &mut self,
        target: StartupScreen,
    ) {
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

    fn commit(
        &mut self,
        screen: StartupScreen,
    ) {
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

fn higher(
    peak: Option<f32>,
    reported: Option<f32>,
) -> Option<f32> {
    match (peak, reported) {
        (Some(peak), Some(reported)) => Some(peak.max(reported)),
        (peak, reported) => peak.or(reported),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::outage::Outage;

    const REVEAL: Duration = Duration::from_millis(250);
    const VISIBLE: Duration = Duration::from_millis(600);
    const RELEASE: Duration = Duration::from_millis(850);

    #[derive(Clone, Copy)]
    enum Move {
        Tick(u64),
        Starting(u64),
        Ready(u64),
        Step(u64, u32, u32),
        NoStep(u64),
        Stopped(u64),
        Recovering(u64),
        HaltedNewer(u64),
    }

    #[derive(Clone, Copy)]
    enum Want {
        Starting,
        Ready,
        Stopped,
        HaltedNewer,
        Migrating(u32, u32, Option<f32>),
    }

    struct Case {
        name: &'static str,
        moves: &'static [Move],
        screen: Want,
        revealed: bool,
        deadline: Option<Duration>,
    }

    const CASES: &[Case] = &[
        Case {
            name: "a start that finishes before the reveal never draws",
            moves: &[Move::Ready(100)],
            screen: Want::Ready,
            revealed: false,
            deadline: None,
        },
        Case {
            name: "the deadline names the reveal before it happens",
            moves: &[],
            screen: Want::Starting,
            revealed: false,
            deadline: Some(REVEAL),
        },
        Case {
            name: "the reveal clears the deadline",
            moves: &[Move::Tick(250)],
            screen: Want::Starting,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "ready after the reveal is held until the screen was visible long enough",
            moves: &[Move::Tick(300), Move::Ready(300)],
            screen: Want::Starting,
            revealed: true,
            deadline: Some(RELEASE),
        },
        Case {
            name: "the held screen stays until the last millisecond",
            moves: &[Move::Tick(300), Move::Ready(300), Move::Tick(849)],
            screen: Want::Starting,
            revealed: true,
            deadline: Some(RELEASE),
        },
        Case {
            name: "the held screen is replaced when it was visible long enough",
            moves: &[Move::Tick(300), Move::Ready(300), Move::Tick(850)],
            screen: Want::Ready,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "a long visible screen is replaced at once",
            moves: &[Move::Tick(5_000), Move::Ready(5_000)],
            screen: Want::Ready,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "only the latest queued screen survives",
            moves: &[
                Move::Tick(300),
                Move::Step(400, 1, 5),
                Move::Ready(500),
                Move::Tick(850),
            ],
            screen: Want::Ready,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "a late tick cannot move time backwards",
            moves: &[Move::Tick(1_000), Move::Tick(10)],
            screen: Want::Starting,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "a failure shows at once and drops the queue",
            moves: &[Move::Tick(300), Move::Ready(300), Move::Stopped(310)],
            screen: Want::Stopped,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "a failure inside the blank time is drawn immediately",
            moves: &[Move::HaltedNewer(50)],
            screen: Want::HaltedNewer,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "leaving a failure shows the next screen at once",
            moves: &[Move::Stopped(2_000), Move::Starting(2_010)],
            screen: Want::Starting,
            revealed: true,
            deadline: None,
        },
        Case {
            name: "progress never goes backwards within a run",
            moves: &[
                Move::Step(300, 1, 5),
                Move::Tick(900),
                Move::Step(1_000, 3, 5),
                Move::Step(1_100, 2, 5),
            ],
            screen: Want::Migrating(3, 5, Some(0.6)),
            revealed: true,
            deadline: None,
        },
        Case {
            name: "a report without a step keeps the last known one",
            moves: &[Move::Step(300, 1, 5), Move::Tick(900), Move::NoStep(1_000)],
            screen: Want::Migrating(1, 5, Some(0.2)),
            revealed: true,
            deadline: None,
        },
        Case {
            name: "a new run starts progress over",
            moves: &[
                Move::Step(300, 1, 5),
                Move::Tick(900),
                Move::Step(1_000, 4, 5),
                Move::Recovering(1_100),
                Move::Starting(1_200),
                Move::Step(1_300, 1, 5),
                Move::Tick(2_000),
            ],
            screen: Want::Migrating(1, 5, Some(0.2)),
            revealed: true,
            deadline: None,
        },
        Case {
            name: "step updates inside migrating are not held",
            moves: &[
                Move::Step(300, 1, 5),
                Move::Tick(900),
                Move::Step(901, 2, 5),
            ],
            screen: Want::Migrating(2, 5, Some(0.4)),
            revealed: true,
            deadline: None,
        },
        Case {
            name: "a plan without steps is indeterminate",
            moves: &[Move::Step(10, 1, 0)],
            screen: Want::Migrating(1, 0, None),
            revealed: false,
            deadline: Some(REVEAL),
        },
    ];

    fn migrating_step(
        index: u32,
        total: u32,
    ) -> MigrationStep {
        MigrationStep::new(format!("step {index}"), index, total)
    }

    fn feed(
        machine: &mut StartupMachine,
        next: Move,
    ) {
        let (millis, input) = match next {
            Move::Tick(millis) => return machine.tick(Duration::from_millis(millis)),
            Move::Starting(millis) => (millis, StartupInput::Starting),
            Move::Ready(millis) => (millis, StartupInput::Ready),
            Move::Step(millis, index, total) => (
                millis,
                StartupInput::Migrating {
                    step: Some(migrating_step(index, total)),
                },
            ),
            Move::NoStep(millis) => (millis, StartupInput::Migrating { step: None }),
            Move::Stopped(millis) => (millis, StartupInput::Unavailable(Outage::Stopped)),
            Move::Recovering(millis) => (millis, recovering()),
            Move::HaltedNewer(millis) => (millis, halted_newer()),
        };
        machine.apply(input, Duration::from_millis(millis));
    }

    fn recovering() -> StartupInput {
        StartupInput::Unavailable(Outage::Recovering {
            attempt: 1,
            reason: "exit 1".to_owned(),
        })
    }

    fn halted_newer() -> StartupInput {
        StartupInput::Halted {
            reason: Some("layout_newer".to_owned()),
            message: "newer".to_owned(),
        }
    }

    fn screen_of(want: Want) -> StartupScreen {
        match want {
            Want::Starting => StartupScreen::Starting,
            Want::Ready => StartupScreen::Ready,
            Want::Stopped => StartupScreen::Unavailable(Outage::Stopped),
            Want::HaltedNewer => StartupScreen::Halted {
                reason: HaltReason::LayoutNewer,
                message: "newer".to_owned(),
            },
            Want::Migrating(index, total, fraction) => StartupScreen::Migrating {
                step: Some(migrating_step(index, total)),
                fraction,
            },
        }
    }

    fn play(moves: &[Move]) -> StartupMachine {
        let timing = StartupTiming::new(REVEAL, VISIBLE);
        let mut machine = StartupMachine::new(timing, Duration::ZERO);
        for next in moves {
            feed(&mut machine, *next);
        }
        machine
    }

    #[test]
    fn the_machine_shows_the_screen_each_script_leads_to() {
        for case in CASES {
            let machine = play(case.moves);
            assert_eq!(machine.screen(), &screen_of(case.screen), "{}", case.name);
            assert_eq!(machine.is_revealed(), case.revealed, "{}", case.name);
            assert_eq!(machine.deadline(), case.deadline, "{}", case.name);
        }
    }
}
