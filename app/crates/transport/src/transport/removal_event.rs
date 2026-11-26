//! What forgetting a paired host logs.

use std::fmt;

use journal::Level;

pub(crate) enum RemovalEvent {
    Unfollowed,
}

impl fmt::Display for RemovalEvent {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unfollowed => write!(f, "no relay is following host removals"),
        }
    }
}

impl journal::Event for RemovalEvent {
    fn source(&self) -> &'static str {
        "transport::transport"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Unfollowed => "host_removal_unfollowed",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Unfollowed => Level::Debug,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn an_event_has_its_name_level_and_message() {
        let event = RemovalEvent::Unfollowed;
        assert_eq!(
            (event.name(), event.level(), event.to_string()),
            (
                "host_removal_unfollowed",
                Level::Debug,
                "no relay is following host removals".to_owned()
            )
        );
    }
}
