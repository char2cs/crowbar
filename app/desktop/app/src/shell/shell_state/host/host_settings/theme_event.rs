//! What handing the theme to the pane web views tells the log.

use std::fmt;

use journal::Level;

pub(super) enum ThemeEvent<'a> {
    Undescribed { error: &'a protocol::Error },
    Unsent { error: &'a panehost::Error },
    Uncoloured { error: &'a panehost::Error },
    Undrawn { error: &'a webview::Error },
}

impl fmt::Display for ThemeEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Undescribed { error } => {
                write!(f, "the theme could not be described to panes error={error}")
            },
            Self::Unsent { error } => write!(f, "sending the theme to panes failed error={error}"),
            Self::Uncoloured { error } => {
                write!(f, "colouring the pane web views failed error={error}")
            },
            Self::Undrawn { error } => {
                write!(
                    f,
                    "recolouring the desktop's menus and dialogs failed error={error}"
                )
            },
        }
    }
}

impl journal::Event for ThemeEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::shell_state::host_settings"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Undescribed { .. } => "theme_undescribed",
            Self::Unsent { .. } => "theme_unsent",
            Self::Uncoloured { .. } => "theme_views_uncoloured",
            Self::Undrawn { .. } => "theme_widgets_undrawn",
        }
    }

    fn level(&self) -> Level {
        Level::Warn
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_theme_event_names_itself_and_reads_as_the_old_log_line() {
        let empty = protocol::Error::EmptyId { kind: "theme" };
        let frame = panehost::Error::Frame(protocol::Error::EmptyId { kind: "theme" });
        let cases = [
            (
                ThemeEvent::Undescribed { error: &empty },
                "theme_undescribed",
                "the theme could not be described to panes error=theme is empty",
            ),
            (
                ThemeEvent::Unsent { error: &frame },
                "theme_unsent",
                "sending the theme to panes failed error=invalid frame from pane: theme is empty",
            ),
            (
                ThemeEvent::Uncoloured { error: &frame },
                "theme_views_uncoloured",
                "colouring the pane web views failed error=invalid frame from pane: theme is empty",
            ),
            (
                ThemeEvent::Undrawn {
                    error: &webview::Error::HostStopped,
                },
                "theme_widgets_undrawn",
                "recolouring the desktop's menus and dialogs failed error=platform thread has stopped",
            ),
        ];
        for (event, name, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Warn, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
