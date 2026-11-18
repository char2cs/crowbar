//! A yes-or-no question the sidebar asks before sending a command.

use crate::shell::chats::RowId;
use crate::shell::command::Command;

const DETACH_BODY: &str = "will move to a detached HEAD, releasing the branch so Crowbar can manage it in its own worktree. Your files are safe — only the working directory's current branch changes; uncommitted changes and commits are preserved.";

/// A confirmation dialog whose accept button sends one command.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Question {
    /// The dialog's title.
    pub title: String,
    /// The explanation under the title.
    pub body: String,
    /// The label of the accept button.
    pub confirm: &'static str,
    /// The command sent when the question is accepted.
    pub command: Command,
}

impl Question {
    /// Asks before detaching the checkout of `branch` so Crowbar can manage it in a worktree.
    /// Names the checkout's `path` when it is known; accepting sends [`Command::Detach`].
    pub fn detach(
        row: RowId,
        branch: &str,
        path: Option<&str>,
    ) -> Self {
        let checkout = path.map_or_else(
            || "The checkout".to_owned(),
            |path| format!("The checkout at {path}"),
        );
        Self {
            title: format!("Detach to manage {branch}"),
            body: format!("{checkout} {DETACH_BODY}"),
            confirm: "Detach",
            command: Command::Detach(row),
        }
    }

    /// Asks before killing the agent running the thread titled `title`.
    /// Accepting sends [`Command::KillAgent`].
    pub fn kill(
        row: RowId,
        title: &str,
    ) -> Self {
        Self {
            title: format!("Kill the agent on “{title}”?"),
            body: "The agent stops immediately and its running turn is lost.".to_owned(),
            confirm: "Kill agent",
            command: Command::KillAgent(row),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_question_names_its_subject_and_sends_its_command() {
        let row = RowId::from("r");
        let detach = Question::detach(row.clone(), "feat", Some("/w/repo"));
        let bare = Question::detach(row.clone(), "feat", None);
        let kill = Question::kill(row.clone(), "Refactor");
        let cases = [
            (
                &detach,
                "Detach to manage feat",
                "The checkout at /w/repo will move",
                Command::Detach(row.clone()),
            ),
            (
                &bare,
                "Detach to manage feat",
                "The checkout will move",
                Command::Detach(row.clone()),
            ),
            (
                &kill,
                "Kill the agent on “Refactor”?",
                "The agent stops",
                Command::KillAgent(row),
            ),
        ];
        for (question, title, body, command) in cases {
            assert_eq!(question.title, title);
            assert!(question.body.starts_with(body), "{}", question.body);
            assert_eq!(question.command, command);
        }
    }
}
