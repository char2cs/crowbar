//! The ordered entries and separators of a file row's menu.

use sidebar::{Command, CopyNote, FileAction, Intent, ShellAnswers};

pub(crate) type FileEntry = (&'static str, FileAction);

#[derive(Debug, Default)]
pub(crate) struct MenuLines(pub(crate) Vec<Option<FileEntry>>);

impl MenuLines {
    pub(crate) fn send(
        &mut self,
        label: &'static str,
        command: Command,
    ) {
        self.run(label, Intent::Send(command));
    }

    pub(crate) fn run(
        &mut self,
        label: &'static str,
        intent: Intent,
    ) {
        self.0.push(Some((label, FileAction::Do(intent))));
    }

    pub(crate) fn copy(
        &mut self,
        label: &'static str,
        (text, note): (String, CopyNote),
    ) {
        self.0.push(Some((label, FileAction::CopyText(text, note))));
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
    let Some((_, action)) = line else {
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
        lines.send("A", Command::UploadFile { path: "a".into() });
        lines.rule();
        lines.copy("B", ("text".into(), CopyNote::Path));
        let labels: Vec<&str> = lines
            .0
            .iter()
            .map(|line| line.as_ref().map_or("-", |(label, _)| *label))
            .collect();
        assert_eq!(labels, ["A", "-", "B"]);
    }
}
