//! What removing a paired host logs when part of it cannot be forgotten.

use std::fmt;

use journal::Level;

use crate::error::Error;
use crate::live::fleet::host_desk::BookError;

pub(crate) enum RosterEvent<'a> {
    KeyKept { error: &'a BookError, host: &'a str },
    NoteKept { error: &'a Error, host: &'a str },
}

impl fmt::Display for RosterEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::KeyKept { error, host } => write!(
                f,
                "the key of a removed host could not be forgotten error={error} host={host:?}"
            ),
            Self::NoteKept { error, host } => write!(
                f,
                "the note of a removed host could not be dropped error={error} host={host:?}"
            ),
        }
    }
}

impl journal::Event for RosterEvent<'_> {
    fn source(&self) -> &'static str {
        "store::live::fleet::host_desk::roster"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::KeyKept { .. } => "roster_key_kept",
            Self::NoteKept { .. } => "roster_note_kept",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::KeyKept { .. } | Self::NoteKept { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn a_kept_key_warns_naming_the_host_and_the_cause() {
        let key = BookError::new(std::io::Error::other("keyring locked"));
        let event = RosterEvent::KeyKept {
            error: &key,
            host: "box",
        };
        let message =
            "the key of a removed host could not be forgotten error=keyring locked host=\"box\"";
        let seen = (event.name(), event.level(), event.to_string());
        assert_eq!(seen, ("roster_key_kept", Level::Warn, message.to_owned()));
    }
}
