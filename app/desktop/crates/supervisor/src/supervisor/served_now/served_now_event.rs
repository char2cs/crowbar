//! What checking a daemon that was just heard from logs when it does not serve.

use std::fmt;
use std::time::Duration;

use journal::{Event, Level};
use tokio::time::error::Elapsed;

use crate::error::Error;

pub(crate) enum ServedNowEvent<'a> {
    NotServing { error: &'a Error },
    NotAnswering { budget: Duration, elapsed: Elapsed },
}

impl fmt::Display for ServedNowEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NotServing { error } => {
                write!(f, "the daemon was heard but does not serve yet: {error}")
            },
            Self::NotAnswering { budget, elapsed } => {
                write!(
                    f,
                    "the daemon was heard but did not answer in {budget:?}: {elapsed}"
                )
            },
        }
    }
}

impl Event for ServedNowEvent<'_> {
    fn source(&self) -> &'static str {
        "supervisor::supervisor::served_now"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NotServing { .. } => "heard_daemon_not_serving",
            Self::NotAnswering { .. } => "heard_daemon_not_answering",
        }
    }

    fn level(&self) -> Level {
        Level::Debug
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn each_served_now_event_has_its_name_level_and_message() {
        let elapsed = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>())
            .await
            .unwrap_err();
        let refused = Error::GaveUp(1);
        let cases = [
            (
                ServedNowEvent::NotServing { error: &refused },
                "heard_daemon_not_serving",
                "the daemon was heard but does not serve yet: daemon did not become healthy in 1 attempts",
            ),
            (
                ServedNowEvent::NotAnswering {
                    budget: Duration::from_secs(2),
                    elapsed,
                },
                "heard_daemon_not_answering",
                "the daemon was heard but did not answer in 2s: deadline has elapsed",
            ),
        ];
        assert_each(cases, Level::Debug);
    }

    fn assert_each<M: fmt::Display>(
        cases: impl IntoIterator<Item = (impl Event, &'static str, M)>,
        level: Level,
    ) {
        for (event, name, shown) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), shown.to_string(), "{name}");
        }
    }
}
