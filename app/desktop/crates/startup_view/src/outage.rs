/// Why the daemon is not serving although it was started.
#[derive(Debug, Clone, PartialEq, Eq)]
#[non_exhaustive]
pub enum Outage {
    /// The watchdog found it wedged and is killing it.
    Unresponsive,
    /// It died and is being started again.
    Recovering {
        /// Which automatic restart this is, from 1.
        attempt: u32,
        /// How the last run ended.
        reason: String,
    },
    /// It keeps dying and is no longer restarted automatically.
    GaveUp {
        /// Version, platform, a summary and the last log lines, ready to show and copy.
        report: String,
    },
    /// It was stopped on purpose.
    Stopped,
}

impl Outage {
    /// Whether the user can ask for another start (the supervisor has stopped trying).
    pub fn is_retryable(&self) -> bool {
        matches!(self, Self::GaveUp { .. } | Self::Stopped)
    }

    /// Whether a restart is under way without the user doing anything.
    pub fn is_restarting(&self) -> bool {
        matches!(self, Self::Unresponsive | Self::Recovering { .. })
    }

    /// The headline of the outage screen.
    pub fn headline(&self) -> &'static str {
        match self {
            Self::Unresponsive => "The background service is not responding",
            Self::Recovering { .. } => "Restarting the background service",
            Self::GaveUp { .. } => "The background service keeps stopping",
            Self::Stopped => "The background service was stopped",
        }
    }

    /// One sentence under the headline.
    pub fn explanation(&self) -> String {
        match self {
            Self::Unresponsive => "Crowbar is restarting it. Your work is safe.".to_owned(),
            Self::Recovering { attempt, .. } => {
                format!("Restart attempt {attempt}. Crowbar continues as soon as it answers.")
            },
            Self::GaveUp { .. } => {
                "Crowbar stopped restarting it. Try again, or open the log to see why.".to_owned()
            },
            Self::Stopped => "Start it again to continue.".to_owned(),
        }
    }

    /// The machine detail worth showing in a mono box, when there is one.
    pub fn detail(&self) -> Option<&str> {
        match self {
            Self::Recovering { reason, .. } => Some(reason),
            Self::GaveUp { report } => Some(report),
            Self::Unresponsive | Self::Stopped => None,
        }
    }
}

#[cfg(test)]
#[path = "outage_tests.rs"]
mod tests;
