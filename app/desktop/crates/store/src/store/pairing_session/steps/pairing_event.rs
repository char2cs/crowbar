//! What pairing logs when a redeemed host cannot be kept or its pairing undone.

use std::fmt;

use journal::Level;

use crate::error::Error;
use crate::live::BookError;

pub(crate) enum PairingEvent<'a> {
    Unkept { error: &'a Error, host: &'a str },
    NotUndone { error: &'a BookError },
}

impl fmt::Display for PairingEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Unkept { error, host } => {
                write!(
                    f,
                    "a paired host could not be kept error={error} host={host}"
                )
            },
            Self::NotUndone { error } => write!(f, "the pairing could not be undone error={error}"),
        }
    }
}

impl journal::Event for PairingEvent<'_> {
    fn source(&self) -> &'static str {
        "store::store::pairing_session::steps"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Unkept { .. } => "pairing_unkept",
            Self::NotUndone { .. } => "pairing_not_undone",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::Unkept { .. } | Self::NotUndone { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn each_event_has_its_name_level_and_message() {
        let unkept = Error::Stream("notes unwritable".into());
        let undone = BookError::new(std::io::Error::other("keyring locked"));
        let cases = [
            (
                PairingEvent::Unkept {
                    error: &unkept,
                    host: "box",
                },
                "pairing_unkept",
                "a paired host could not be kept error=shell data: stream: notes unwritable \
                 host=box",
            ),
            (
                PairingEvent::NotUndone { error: &undone },
                "pairing_not_undone",
                "the pairing could not be undone error=keyring locked",
            ),
        ];
        for (event, name, message) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Warn);
            assert_eq!(event.to_string(), message);
        }
    }
}
