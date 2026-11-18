use crate::app_phase::AppPhase;

use crate::halt_reason::HaltReason;
use crate::migration_step::MigrationStep;
use crate::outage::Outage;

/// What the full-screen view shows right now.
#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum StartupScreen {
    /// Waiting for the daemon to answer.
    Starting,
    /// The daemon upgrades its data.
    Migrating {
        /// The latest step reported.
        step: Option<MigrationStep>,
        /// Overall progress, never lower than an earlier value; `None` while unknown.
        fraction: Option<f32>,
    },
    /// The daemon refused to start.
    Halted {
        /// Why.
        reason: HaltReason,
        /// What the daemon said.
        message: String,
    },
    /// The daemon is not serving.
    Unavailable(Outage),
    /// The daemon serves; the view hands over to the layout.
    Ready,
}

impl StartupScreen {
    /// Whether this screen is part of a normal start (as opposed to a failure or the end).
    pub fn is_waiting(&self) -> bool {
        matches!(self, Self::Starting | Self::Migrating { .. })
    }

    /// Whether this screen reports a failure that must show at once.
    pub fn is_failure(&self) -> bool {
        matches!(self, Self::Halted { .. } | Self::Unavailable(_))
    }

    /// The headline, also the window title.
    pub fn headline(&self) -> &'static str {
        match self {
            Self::Starting => "Starting Crowbar",
            Self::Migrating { .. } => "Upgrading your data",
            Self::Halted { .. } => "Crowbar could not start",
            Self::Unavailable(outage) => outage.headline(),
            Self::Ready => "Crowbar is ready",
        }
    }

    /// The text a screen reader should announce for this screen.
    pub fn announcement(&self) -> String {
        match self {
            Self::Starting => "Starting Crowbar. Waiting for the background service.".to_owned(),
            Self::Migrating { step, fraction } => {
                migrating_announcement(self.headline(), step.as_ref(), *fraction)
            },
            Self::Halted { reason, message } => {
                format!("{}. {} {message}", self.headline(), reason.explanation())
            },
            Self::Unavailable(outage) => format!("{}. {}", self.headline(), outage.explanation()),
            Self::Ready => self.headline().to_owned(),
        }
    }

    /// The app phase this screen corresponds to.
    pub fn phase(&self) -> AppPhase {
        match self {
            Self::Starting => AppPhase::Starting,
            Self::Migrating { step, fraction } => AppPhase::Migrating {
                state: step.as_ref().map_or_default(MigrationStep::label),
                progress: *fraction,
            },
            Self::Halted { message, .. } => AppPhase::DaemonUnavailable {
                reason: message.clone(),
            },
            Self::Unavailable(outage) => AppPhase::DaemonUnavailable {
                reason: outage.explanation(),
            },
            Self::Ready => AppPhase::Ready,
        }
    }
}

fn migrating_announcement(
    headline: &str,
    step: Option<&MigrationStep>,
    fraction: Option<f32>,
) -> String {
    let parts = [
        Some(headline.to_owned()),
        step.map(MigrationStep::label),
        fraction.map(|fraction| format!("{} percent", percent(fraction))),
    ];
    let parts: Vec<String> = parts.into_iter().flatten().collect();
    format!("{}.", parts.join(". "))
}

const PERCENT_SCALE: f32 = 100.0;
const PERCENT_MAX: u8 = 100;

pub(crate) fn percent(fraction: f32) -> u8 {
    let scaled = (fraction.clamp(0.0, 1.0) * PERCENT_SCALE).round();
    (0..=PERCENT_MAX)
        .find(|percent| f32::from(*percent) >= scaled)
        .unwrap_or(PERCENT_MAX)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn migrating(
        step: Option<MigrationStep>,
        fraction: Option<f32>,
    ) -> StartupScreen {
        StartupScreen::Migrating { step, fraction }
    }

    #[test]
    fn only_ready_shows_the_layout() {
        let screens = [
            StartupScreen::Starting,
            migrating(None, None),
            StartupScreen::Halted {
                reason: HaltReason::StepFailed,
                message: "boom".to_owned(),
            },
            StartupScreen::Unavailable(Outage::Stopped),
            StartupScreen::Ready,
        ];
        let shown: Vec<bool> = screens
            .iter()
            .map(|screen| screen.phase().shows_layout())
            .collect();
        assert_eq!(shown, [false, false, false, false, true]);
    }

    #[test]
    fn migrating_phase_carries_the_step_and_progress() {
        let step = MigrationStep::new("Rebuild index", 2, 5);
        let phase = migrating(Some(step), Some(0.4)).phase();
        assert_eq!(
            phase,
            AppPhase::Migrating {
                state: "Step 2 of 5: Rebuild index".to_owned(),
                progress: Some(0.4)
            }
        );
    }

    #[test]
    fn announcements_name_the_state_step_and_percent() {
        let step = MigrationStep::new("Rebuild index", 2, 5);
        assert_eq!(
            migrating(Some(step), Some(0.4)).announcement(),
            "Upgrading your data. Step 2 of 5: Rebuild index. 40 percent."
        );
        assert_eq!(migrating(None, None).announcement(), "Upgrading your data.");
        let halted = StartupScreen::Halted {
            reason: HaltReason::LayoutNewer,
            message: "layout 9 is newer than 7".to_owned(),
        };
        assert!(
            halted
                .announcement()
                .starts_with("Crowbar could not start. ")
        );
        assert!(halted.announcement().ends_with("layout 9 is newer than 7"));
    }

    #[test]
    fn waiting_and_failure_are_exclusive() {
        let screens = [
            (StartupScreen::Starting, true, false),
            (migrating(None, None), true, false),
            (
                StartupScreen::Unavailable(Outage::Unresponsive),
                false,
                true,
            ),
            (StartupScreen::Ready, false, false),
        ];
        for (screen, waiting, failure) in screens {
            assert_eq!(screen.is_waiting(), waiting, "{screen:?}");
            assert_eq!(screen.is_failure(), failure, "{screen:?}");
        }
    }

    #[test]
    fn percent_rounds_and_clamps() {
        const CASES: &[(f32, u8)] = &[(0.0, 0), (0.404, 40), (0.996, 100), (7.0, 100), (-1.0, 0)];
        for &(fraction, expected) in CASES {
            assert_eq!(percent(fraction), expected, "{fraction}");
        }
    }
}
