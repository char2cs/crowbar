use std::fmt;
use std::path::Path;

use journal::{Event, Level};

pub(crate) enum DaemonLogEvent<'a> {
    Unwritable {
        path: &'a Path,
        error: &'a std::io::Error,
    },
    FlushUnfinished,
}

impl fmt::Display for DaemonLogEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unwritable { path, error } => write!(f, "daemon log {}: {error}", path.display()),
            Self::FlushUnfinished => {
                f.write_str("the daemon log writer ended before a flush finished")
            },
        }
    }
}

impl Event for DaemonLogEvent<'_> {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::supervision::log::daemon_log"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Unwritable { .. } => "daemon_log_unwritable",
            Self::FlushUnfinished => "daemon_log_flush_unfinished",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Unwritable { .. } => Level::Warn,
            Self::FlushUnfinished => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::events::assert_events;

    #[test]
    fn an_unwritable_log_is_a_warning_naming_the_file() {
        let denied = std::io::Error::from(std::io::ErrorKind::PermissionDenied);
        let event = DaemonLogEvent::Unwritable {
            path: Path::new("/var/log/d.log"),
            error: &denied,
        };
        assert_events([(
            event,
            "daemon_log_unwritable",
            Level::Warn,
            "daemon log /var/log/d.log: permission denied",
        )]);
    }
}
