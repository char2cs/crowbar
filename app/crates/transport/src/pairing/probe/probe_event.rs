//! What the unpinned fingerprint probe logs.

use std::{fmt, io};

use journal::Level;

pub(crate) enum ProbeEvent<'a> {
    UncleanClose { error: &'a io::Error },
}

impl fmt::Display for ProbeEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::UncleanClose { error } => {
                write!(f, "probe connection did not close cleanly: {error}")
            },
        }
    }
}

impl journal::Event for ProbeEvent<'_> {
    fn source(&self) -> &'static str {
        "transport::pairing::probe"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::UncleanClose { .. } => "probe_unclean_close",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::UncleanClose { .. } => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn an_event_has_its_name_level_and_message() {
        let event = ProbeEvent::UncleanClose {
            error: &io::Error::other("reset"),
        };
        assert_eq!(
            (event.name(), event.level(), event.to_string()),
            (
                "probe_unclean_close",
                Level::Debug,
                "probe connection did not close cleanly: reset".to_owned()
            )
        );
    }
}
