//! What the handle logs while it stops the supervision task.

use std::fmt;

use journal::{Event, Level};
use tokio::task::JoinError;

pub(crate) enum SupervisorEvent<'a> {
    StopUnconfirmed,
    TaskFailed { error: &'a JoinError },
}

impl fmt::Display for SupervisorEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::StopUnconfirmed => {
                f.write_str("the supervision task ended without confirming the stop")
            },
            Self::TaskFailed { error } => write!(f, "the supervision task failed: {error}"),
        }
    }
}

impl Event for SupervisorEvent<'_> {
    fn source(&self) -> &'static str {
        "supervisor::supervisor"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::StopUnconfirmed => "supervision_stop_unconfirmed",
            Self::TaskFailed { .. } => "supervision_task_failed",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::StopUnconfirmed => Level::Debug,
            Self::TaskFailed { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn each_supervisor_event_has_its_name_level_and_message() {
        let task = tokio::spawn(std::future::pending::<()>());
        task.abort();
        let error = task.await.unwrap_err();
        let failed = format!("the supervision task failed: {error}");
        let cases = [
            (
                SupervisorEvent::StopUnconfirmed,
                "supervision_stop_unconfirmed",
                Level::Debug,
                "the supervision task ended without confirming the stop",
            ),
            (
                SupervisorEvent::TaskFailed { error: &error },
                "supervision_task_failed",
                Level::Warn,
                failed.as_str(),
            ),
        ];
        assert_each(cases);
    }

    fn assert_each<M: fmt::Display>(
        cases: impl IntoIterator<Item = (impl Event, &'static str, Level, M)>
    ) {
        for (event, name, level, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown.to_string(), "{name}");
        }
    }
}
