use std::fmt;

use journal::Level;

pub(super) enum NewChatEvent<'a> {
    NeedsProject,
    Refused { error: &'a sidebar::Error },
}

impl fmt::Display for NewChatEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NeedsProject => write!(f, "a new chat needs a project to be selected first"),
            Self::Refused { error } => write!(f, "the new chat was refused error={error}"),
        }
    }
}

impl journal::Event for NewChatEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::shell_view::shell_commands"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NeedsProject => "new_chat_needs_project",
            Self::Refused { .. } => "new_chat_refused",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::NeedsProject => Level::Info,
            Self::Refused { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_new_chat_event_names_itself_and_reads_as_the_old_log_line() {
        let refused = sidebar::Error::Refused {
            reason: "offline".into(),
        };
        let cases = [
            (
                NewChatEvent::NeedsProject,
                "new_chat_needs_project",
                Level::Info,
                "a new chat needs a project to be selected first",
            ),
            (
                NewChatEvent::Refused { error: &refused },
                "new_chat_refused",
                Level::Warn,
                "the new chat was refused error=sidebar: command refused: offline",
            ),
        ];
        for (event, name, level, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
