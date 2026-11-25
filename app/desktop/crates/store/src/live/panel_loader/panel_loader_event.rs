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
    fn each_event_has_its_name_level_and_message() {
        let error = daemonapi::Error::Stream("reset".into());
        let cases = [(
            PanelLoaderEvent::StatusUnread {
                chat: "c",
                error: &error,
            },
            "panel_loader_status_unread",
            Level::Warn,
            format!("git status could not be read chat=\"c\" error={error}"),
        )];
        for (event, name, level, message) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level);
            assert_eq!(event.to_string(), message);
        }
    }
}
