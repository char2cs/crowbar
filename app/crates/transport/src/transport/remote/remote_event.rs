//! What forwarding a request to a paired remote host logs.

use std::fmt;

use journal::Level;

use crate::error::Error;

pub(crate) enum RemoteEvent<'a> {
    ExchangeFailed { error: &'a Error },
}

impl fmt::Display for RemoteEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::ExchangeFailed { error } => write!(f, "remote exchange failed: {error}"),
        }
    }
}

impl journal::Event for RemoteEvent<'_> {
    fn source(&self) -> &'static str {
        "transport::transport::remote"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::ExchangeFailed { .. } => "remote_exchange_failed",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::ExchangeFailed { .. } => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn an_event_has_its_name_level_and_message() {
        let event = RemoteEvent::ExchangeFailed {
            error: &Error::TooLarge,
        };
        assert_eq!(
            (event.name(), event.level(), event.to_string()),
            (
                "remote_exchange_failed",
                Level::Debug,
                "remote exchange failed: response too large".to_owned()
            )
        );
    }
}
