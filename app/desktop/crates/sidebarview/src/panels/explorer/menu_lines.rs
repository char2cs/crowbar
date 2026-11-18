//! The ordered entries and separators of a file row's menu.

use ui::IconName;

use sidebar::{Command, FileAction, Intent, ShellAnswers};

pub(crate) type FileEntry = (&'static str, IconName, FileAction);

#[derive(Debug, Default)]
pub(crate) struct MenuLines(pub(crate) Vec<Option<FileEntry>>);

impl MenuLines {
    pub(crate) fn send(
        &mut self,
        (label, icon): (&'static str, IconName),
        command: Command,
    ) {
        self.run((label, icon), Intent::Send(command));
    }

    pub(crate) fn run(
        &mut self,
        (label, icon): (&'static str, IconName),
        intent: Intent,
    ) {
        self.0.push(Some((label, icon, FileAction::Do(intent))));
    }

    pub(crate) fn copy(
        &mut self,
        (label, icon): (&'static str, IconName),
        text: String,
    ) {
        self.0.push(Some((label, icon, FileAction::CopyText(text))));
    }

    pub(crate) fn rule(&mut self) {
        self.0.push(None);
    }

    pub(crate) fn retain_served(
        &mut self,
        answers: &ShellAnswers,
    ) {
        self.0.retain(|line| is_served(line.as_ref(), answers));
        self.0
            .dedup_by(|current, previous| current.is_none() && previous.is_none());
        if self.0.first().is_some_and(Option::is_none) {
            self.0.remove(0);
        }
        if self.0.last().is_some_and(Option::is_none) {
            self.0.pop();
        }
    }
}

fn is_served(
    line: Option<&FileEntry>,
    answers: &ShellAnswers,
) -> bool {
    let Some((_, _, action)) = line else {
        return true;
    };
    action
        .command()
        .is_none_or(|command| answers.serves(&command))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lines_keep_their_order_and_separators() {
        let mut lines = MenuLines::default();
        lines.send(("A", IconName::Copy), Command::AddFolderToWorkspace);
        lines.rule();
        lines.copy(("B", IconName::Copy), "text".into());
        let labels: Vec<&str> = lines
            .0
            .iter()
            .map(|line| line.as_ref().map_or("-", |(label, _, _)| *label))
            .collect();
        assert_eq!(labels, ["A", "-", "B"]);
    }
}
