//! What the watchdog logs as it probes the daemon and reports a wedge.

use std::fmt;

use journal::{Event, Level};

pub(crate) enum WatchdogEvent {
    NotWedged {
        suspended: bool,
        answering: bool,
        busy: bool,
    },
    HealthyUnread,
    OutOfDescriptors,
    WedgeUnheard,
}

impl fmt::Display for WatchdogEvent {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NotWedged {
                suspended,
                answering,
                busy,
            } => write!(
                f,
                "daemon failed its probes but is not wedged (suspended: {suspended}, answering: {answering}, busy: {busy})"
            ),
            Self::HealthyUnread => f.write_str("an earlier healthy probe is still to be read"),
            Self::OutOfDescriptors => {
                f.write_str("out of file descriptors, cannot dial the daemon; not restarting it")
            },
            Self::WedgeUnheard => f.write_str("the supervisor stopped listening for wedge reports"),
        }
    }
}

impl Event for WatchdogEvent {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::monitoring::watchdog"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NotWedged { .. } => "daemon_not_wedged",
            Self::HealthyUnread => "healthy_probe_unread",
            Self::OutOfDescriptors => "descriptors_exhausted",
            Self::WedgeUnheard => "wedge_report_unheard",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::NotWedged { .. } => Level::Warn,
            Self::HealthyUnread => Level::Trace,
            Self::OutOfDescriptors => Level::Error,
            Self::WedgeUnheard => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (WatchdogEvent, &'static str, Level, &'static str);

    const CASES: [Case; 4] = [
        (
            WatchdogEvent::NotWedged {
                suspended: false,
                answering: true,
                busy: false,
            },
            "daemon_not_wedged",
            Level::Warn,
            "daemon failed its probes but is not wedged (suspended: false, answering: true, busy: false)",
        ),
        (
            WatchdogEvent::HealthyUnread,
            "healthy_probe_unread",
            Level::Trace,
            "an earlier healthy probe is still to be read",
        ),
        (
            WatchdogEvent::OutOfDescriptors,
            "descriptors_exhausted",
            Level::Error,
            "out of file descriptors, cannot dial the daemon; not restarting it",
        ),
        (
            WatchdogEvent::WedgeUnheard,
            "wedge_report_unheard",
            Level::Debug,
            "the supervisor stopped listening for wedge reports",
        ),
    ];

    #[test]
    fn each_watchdog_event_has_its_name_level_and_message() {
        for (event, name, level, shown) in CASES {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown, "{name}");
        }
    }
}
