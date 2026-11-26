//! What revealing a workspace path in the file manager tells the log.

use std::fmt;

use journal::Level;

pub(super) enum RevealEvent<'a> {
    NeedsWorkspace,
    LeftWorkspace { path: &'a str },
    Unopened { error: &'a platform::Error },
}

impl fmt::Display for RevealEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NeedsWorkspace => write!(f, "revealing needs a workspace on this computer"),
            Self::LeftWorkspace { path } => {
                write!(f, "a reveal path left the workspace path={path:?}")
            },
            Self::Unopened { error } => {
                write!(f, "the file manager could not be opened error={error}")
            },
        }
    }
}

impl journal::Event for RevealEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::shell_view::pane_commands::files"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NeedsWorkspace => "reveal_needs_workspace",
            Self::LeftWorkspace { .. } => "reveal_left_workspace",
            Self::Unopened { .. } => "reveal_file_manager_unopened",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::NeedsWorkspace => Level::Info,
            Self::LeftWorkspace { .. } | Self::Unopened { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_reveal_event_names_itself_and_reads_as_the_old_log_line() {
        let refused = platform::Error::Refused {
            target: "x".into(),
            reason: "no",
        };
        let cases = [
            (
                RevealEvent::NeedsWorkspace,
                "reveal_needs_workspace",
                Level::Info,
                "revealing needs a workspace on this computer",
            ),
            (
                RevealEvent::LeftWorkspace { path: "../up" },
                "reveal_left_workspace",
                Level::Warn,
                "a reveal path left the workspace path=\"../up\"",
            ),
            (
                RevealEvent::Unopened { error: &refused },
                "reveal_file_manager_unopened",
                Level::Warn,
                "the file manager could not be opened error=refusing to open \"x\": no",
            ),
        ];
        for (event, name, level, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
