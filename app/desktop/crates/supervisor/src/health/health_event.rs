//! What the health wait logs about an attempt that got no usable answer.

use std::fmt;
use std::time::Duration;

use journal::{Event, Level};
use tokio::time::error::Elapsed;

use crate::error::Error;

pub(crate) enum HealthEvent<'a> {
    Unreadable { error: &'a Error },
    Unanswered { after: Duration, elapsed: Elapsed },
}

impl fmt::Display for HealthEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unreadable { error } => write!(f, "daemon health: {error}"),
            Self::Unanswered { after, elapsed } => {
                write!(f, "daemon health: no answer in {after:?} ({elapsed})")
            },
        }
    }
}

impl Event for HealthEvent<'_> {
    fn source(&self) -> &'static str {
        "supervisor::health"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Unreadable { .. } => "health_unreadable",
            Self::Unanswered { .. } => "health_unanswered",
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
    async fn each_health_event_has_its_name_level_and_message() {
        let elapsed = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>())
            .await
            .unwrap_err();
        let error = Error::GaveUp(3);
        let cases = [
            (
                HealthEvent::Unreadable { error: &error },
                "health_unreadable",
                "daemon health: daemon did not become healthy in 3 attempts",
            ),
            (
                HealthEvent::Unanswered {
                    after: Duration::from_secs(2),
                    elapsed,
                },
                "health_unanswered",
                "daemon health: no answer in 2s (deadline has elapsed)",
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
