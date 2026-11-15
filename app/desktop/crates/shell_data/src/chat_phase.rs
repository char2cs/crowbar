//! Where a chat's runner is in its life.

/// The runner's phase as the daemon reports it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ChatPhase {
    /// No runner.
    Dormant,
    /// A runner is starting.
    Starting,
    /// A runner is live.
    Live,
    /// The runner is being replaced.
    Switching,
    /// The runner is stopping.
    Stopping,
}

impl ChatPhase {
    pub(crate) fn from_wire(
        text: &str,
        runner: &str,
    ) -> Self {
        match (text, runner.is_empty()) {
            ("dormant", _) => Self::Dormant,
            ("starting", _) => Self::Starting,
            ("switching", _) => Self::Switching,
            ("stopping", _) => Self::Stopping,
            ("live", _) | (_, false) => Self::Live,
            (_, true) => Self::Dormant,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_wire_word_wins_and_a_missing_one_follows_the_runner() {
        const CASES: &[(&str, &str, ChatPhase)] = &[
            ("dormant", "", ChatPhase::Dormant),
            ("starting", "", ChatPhase::Starting),
            ("live", "r", ChatPhase::Live),
            ("switching", "r", ChatPhase::Switching),
            ("stopping", "r", ChatPhase::Stopping),
            ("", "r", ChatPhase::Live),
            ("", "", ChatPhase::Dormant),
            ("odd", "r", ChatPhase::Live),
        ];
        for &(text, runner, expected) in CASES {
            assert_eq!(ChatPhase::from_wire(text, runner), expected, "{text:?}");
        }
    }
}
