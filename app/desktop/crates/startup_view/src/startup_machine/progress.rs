//! How inputs map to screens and how migration progress is kept.

use super::StartupMachine;
use crate::halt_reason::HaltReason;
use crate::migration_step::MigrationStep;
use crate::startup_input::StartupInput;
use crate::startup_screen::StartupScreen;

impl StartupMachine {
    pub(crate) fn screen_for(
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

    pub(crate) fn halted(
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

    pub(crate) fn migrating(
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

    pub(crate) fn is_stale(
        &self,
        reported: &MigrationStep,
    ) -> bool {
        self.step
            .as_ref()
            .is_some_and(|current| reported.index() < current.index())
    }

    pub(crate) fn forget_run(&mut self) {
        self.step = None;
        self.peak = None;
    }
}

pub(crate) fn higher(
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

    #[test]
    fn the_peak_never_falls() {
        type Case = (Option<f32>, Option<f32>, Option<f32>);
        const CASES: &[Case] = &[
            (None, None, None),
            (None, Some(0.4), Some(0.4)),
            (Some(0.6), None, Some(0.6)),
            (Some(0.6), Some(0.4), Some(0.6)),
            (Some(0.4), Some(0.6), Some(0.6)),
        ];
        for &(peak, reported, expected) in CASES {
            assert_eq!(higher(peak, reported), expected, "{peak:?} {reported:?}");
        }
    }
}
