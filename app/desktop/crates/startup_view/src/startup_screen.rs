use protocol::AppPhase;

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

    /// The protocol phase this screen corresponds to.
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

/// `0.4` as `40`.
#[expect(
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    reason = "the fraction is clamped to 0..=1 so the percentage fits in u8"
)]
pub(crate) fn percent(fraction: f32) -> u8 {
    (fraction.clamp(0.0, 1.0) * 100.0).round() as u8
}

#[cfg(test)]
#[path = "startup_screen_tests.rs"]
mod tests;
