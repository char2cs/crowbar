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
mod tests {
    use super::*;

    struct Case {
        make: fn() -> Outage,
        retryable: bool,
        restarting: bool,
        detail: Option<&'static str>,
        explanation: &'static str,
    }

    const CASES: &[Case] = &[
        Case {
            make: || Outage::Unresponsive,
            retryable: false,
            restarting: true,
            detail: None,
            explanation: "restarting it",
        },
        Case {
            make: recovering,
            retryable: false,
            restarting: true,
            detail: Some("exit status 1"),
            explanation: "attempt 3",
        },
        Case {
            make: gave_up,
            retryable: true,
            restarting: false,
            detail: Some("crowbar 1.0"),
            explanation: "stopped restarting",
        },
        Case {
            make: || Outage::Stopped,
            retryable: true,
            restarting: false,
            detail: None,
            explanation: "Start it again",
        },
    ];

    fn recovering() -> Outage {
        Outage::Recovering {
            attempt: 3,
            reason: "exit status 1".to_owned(),
        }
    }

    fn gave_up() -> Outage {
        Outage::GaveUp {
            report: "crowbar 1.0".to_owned(),
        }
    }

    #[test]
    fn each_outage_offers_retry_names_its_detail_and_explains_itself() {
        for case in CASES {
            let outage = (case.make)();
            assert_eq!(outage.is_retryable(), case.retryable, "{outage:?}");
            assert_eq!(outage.is_restarting(), case.restarting, "{outage:?}");
            assert_eq!(outage.detail(), case.detail, "{outage:?}");
            assert!(!outage.headline().is_empty(), "{outage:?}");
            assert!(
                outage.explanation().contains(case.explanation),
                "{outage:?}"
            );
        }
    }
}
