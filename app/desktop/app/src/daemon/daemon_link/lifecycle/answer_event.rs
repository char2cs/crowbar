use std::fmt;

use journal::Level;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) enum AnswerEvent {
    RelaunchUnheard,
    RestartUnheard,
}

impl fmt::Display for AnswerEvent {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        let what = match self {
            Self::RelaunchUnheard => "relaunch",
            Self::RestartUnheard => "restart",
        };
        write!(f, "nobody waits on the daemon's {what}")
    }
}

impl journal::Event for AnswerEvent {
    fn source(&self) -> &'static str {
        "crowbar::daemon_link::lifecycle"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::RelaunchUnheard => "daemon_relaunch_unheard",
            Self::RestartUnheard => "daemon_restart_unheard",
        }
    }

    fn level(&self) -> Level {
        Level::Debug
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_unheard_answer_names_itself_and_reads_as_the_old_log_line() {
        let cases = [
            (
                AnswerEvent::RelaunchUnheard,
                "daemon_relaunch_unheard",
                "nobody waits on the daemon's relaunch",
            ),
            (
                AnswerEvent::RestartUnheard,
                "daemon_restart_unheard",
                "nobody waits on the daemon's restart",
            ),
        ];
        for (event, name, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Debug, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
