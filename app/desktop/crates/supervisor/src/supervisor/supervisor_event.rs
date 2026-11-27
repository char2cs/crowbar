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
    use crate::test_support::events::assert_events;

    #[tokio::test]
    async fn a_failed_task_is_a_warning_naming_the_failure() {
        let task = tokio::spawn(std::future::pending::<()>());
        task.abort();
        let error = task.await.unwrap_err();
        let failed = format!("the supervision task failed: {error}");
        let event = SupervisorEvent::TaskFailed { error: &error };
        assert_events([(event, "supervision_task_failed", Level::Warn, failed)]);
    }
}
