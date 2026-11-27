use std::{fmt, io};

use journal::Level;
use tokio::time::error::Elapsed;

pub(crate) enum BusyEvent<'a> {
    NotDelivered { error: &'a io::Error },
    NotTaken { elapsed: &'a Elapsed },
}

impl fmt::Display for BusyEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NotDelivered { error } => {
                write!(
                    f,
                    "relay is busy and the refusal was not delivered: {error}"
                )
            },
            Self::NotTaken { elapsed } => {
                write!(
                    f,
                    "relay is busy and the refusal was not taken in time: {elapsed}"
                )
            },
        }
    }
}

impl journal::Event for BusyEvent<'_> {
    fn source(&self) -> &'static str {
        "transport::relay::accept_loop"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NotDelivered { .. } => "relay_busy_refusal_not_delivered",
            Self::NotTaken { .. } => "relay_busy_refusal_not_taken",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::NotDelivered { .. } | Self::NotTaken { .. } => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use journal::Event;

    use super::*;

    #[tokio::test]
    async fn each_event_has_its_name_level_and_message() {
        let error = io::Error::other("reset");
        let late = tokio::time::timeout(Duration::ZERO, std::future::pending::<()>()).await;
        let elapsed = late.expect_err("a pending future outlasts a zero timeout");
        let cases = [
            (
                BusyEvent::NotDelivered { error: &error },
                "relay_busy_refusal_not_delivered",
                Level::Debug,
                "relay is busy and the refusal was not delivered: reset".to_owned(),
            ),
            (
                BusyEvent::NotTaken { elapsed: &elapsed },
                "relay_busy_refusal_not_taken",
                Level::Debug,
                format!("relay is busy and the refusal was not taken in time: {elapsed}"),
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
