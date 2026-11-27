use std::fmt;

use journal::Level;
use protocol::PaneId;

pub(super) enum PageAnswerEvent<'a> {
    NotAwaited { id: &'a str },
    OtherTab { id: &'a str, pane: &'a PaneId },
}

impl fmt::Display for PageAnswerEvent<'_> {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::NotAwaited { id } => {
                write!(f, "a page answered a request nobody awaits id={id:?}")
            },
            Self::OtherTab { id, pane } => {
                write!(f, "a page answered for another tab id={id:?} pane={pane}")
            },
        }
    }
}

impl journal::Event for PageAnswerEvent<'_> {
    fn source(&self) -> &'static str {
        "crowbar::shell_state::agent_ops"
    }

    fn name(&self) -> &'static str {
        match self {
            Self::NotAwaited { .. } => "page_answer_not_awaited",
            Self::OtherTab { .. } => "page_answer_for_other_tab",
        }
    }

    fn level(&self) -> Level {
        match self {
            Self::NotAwaited { .. } => Level::Debug,
            Self::OtherTab { .. } => Level::Warn,
        }
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_page_answer_event_names_itself_and_reads_as_the_old_log_line() {
        let pane = PaneId::new("p2").unwrap();
        let cases = [
            (
                PageAnswerEvent::NotAwaited { id: "r1" },
                "page_answer_not_awaited",
                Level::Debug,
                "a page answered a request nobody awaits id=\"r1\"",
            ),
            (
                PageAnswerEvent::OtherTab {
                    id: "r1",
                    pane: &pane,
                },
                "page_answer_for_other_tab",
                Level::Warn,
                "a page answered for another tab id=\"r1\" pane=p2",
            ),
        ];
        for (event, name, level, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), level, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
