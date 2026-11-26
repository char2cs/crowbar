//! What filling the Changes panel logs when the chat's git status cannot be read.

use std::fmt;

use journal::Level;

pub(crate) enum PanelLoaderEvent<'a> {
    StatusUnread {
        chat: &'a str,
        error: &'a daemonapi::Error,
    },
}

impl fmt::Display for PanelLoaderEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::StatusUnread { chat, error } => {
                write!(
                    f,
                    "git status could not be read chat={chat:?} error={error}"
                )
            },
        }
    }
}

impl journal::Event for PanelLoaderEvent<'_> {
    fn source(&self) -> &'static str {
        "store::live::panel_loader"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::StatusUnread { .. } => "panel_loader_status_unread",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::StatusUnread { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event;

    use super::*;

    #[test]
    fn an_unread_status_warns_naming_the_chat_and_the_cause() {
        let error = daemonapi::Error::Stream("reset".into());
        let event = PanelLoaderEvent::StatusUnread {
            chat: "c",
            error: &error,
        };
        let message = format!("git status could not be read chat=\"c\" error={error}");
        let seen = (event.name(), event.level(), event.to_string());
        assert_eq!(seen, ("panel_loader_status_unread", Level::Warn, message));
    }
}
