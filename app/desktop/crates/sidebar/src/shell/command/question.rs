use crate::shell::chats::RowId;
use crate::shell::command::{Command, Stakes};

const KILL_BODY: &str =
    "It stops now, even mid-turn. The conversation is kept and resumes when reopened.";
const DETACH_BODY: &str = "will move to a detached HEAD, releasing the branch so Crowbar can manage it in its own worktree. Your files are safe — only the working directory's current branch changes; uncommitted changes and commits are preserved.";

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Question {
    pub title: String,
    pub body: String,
    pub cancel: &'static str,
    pub confirm: &'static str,
    pub stakes: Stakes,
    pub command: Command,
}

impl Question {
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
            cancel: "Cancel",
            confirm: "Detach",
            stakes: Stakes::Disruptive,
            command: Command::Detach(row),
        }
    }

    pub fn kill(
        row: RowId,
        title: &str,
    ) -> Self {
        Self {
            title: format!("Kill the agent on “{title}”?"),
            body: KILL_BODY.to_owned(),
            cancel: "Keep running",
            confirm: "Kill agent",
            stakes: Stakes::Destructive,
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
                "It stops now, even mid-turn.",
                Command::KillAgent(row),
            ),
        ];
        for (question, title, body, command) in cases {
            assert_eq!(question.title, title);
            assert!(question.body.starts_with(body), "{}", question.body);
            assert_eq!(question.command, command);
        }
        let answers = |q: &Question| (q.cancel, q.confirm, q.stakes);
        assert_eq!(answers(&detach), ("Cancel", "Detach", Stakes::Disruptive));
        assert_eq!(
            answers(&kill),
            ("Keep running", "Kill agent", Stakes::Destructive)
        );
    }
}
