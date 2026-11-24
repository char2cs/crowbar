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
