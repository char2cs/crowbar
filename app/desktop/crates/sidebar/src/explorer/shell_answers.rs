//! What the shell answers about the Files panel, asked lazily when a menu is built.

use std::fmt;
use std::rc::Rc;

use crate::shell::Command;
use crate::shell::Source;

/// A handle on the shell's sidebar data for menu questions; the default has no shell.
#[derive(Clone, Default)]
pub struct ShellAnswers {
    data: Option<Rc<dyn Source>>,
}

impl ShellAnswers {
    /// Answers from `data`, sharing it.
    pub fn of(data: &Rc<dyn Source>) -> Self {
        Self {
            data: Some(Rc::clone(data)),
        }
    }

    /// Whether the shell carries out `command`; without a shell, every command is served.
    pub fn serves(
        &self,
        command: &Command,
    ) -> bool {
        self.data.as_ref().is_none_or(|data| data.serves(command))
    }

    /// Whether `path` is one of several folders of the workspace, which offers Remove From
    /// Workspace; without a shell, none is.
    pub fn can_remove_folder(
        &self,
        path: &str,
    ) -> bool {
        self.data
            .as_ref()
            .is_some_and(|data| data.can_remove_folder(path))
    }
}

impl fmt::Debug for ShellAnswers {
    fn fmt(
        &self,
        formatter: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        formatter
            .debug_struct("ShellAnswers")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn without_a_shell_everything_is_served_and_nothing_is_removable() {
        let answers = ShellAnswers::default();
        assert!(answers.serves(&Command::UploadFile { path: "src".into() }));
        assert!(!answers.can_remove_folder("src"));
    }
}
