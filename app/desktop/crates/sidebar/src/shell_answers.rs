//! What the shell answers about the Files panel, asked lazily when a menu is built.

use std::fmt;
use std::rc::Rc;

use crate::command::Command;
use crate::sidebar_data::SidebarData;

#[derive(Clone, Default)]
pub(crate) struct ShellAnswers {
    data: Option<Rc<dyn SidebarData>>,
}

impl ShellAnswers {
    pub(crate) fn of(data: &Rc<dyn SidebarData>) -> Self {
        Self {
            data: Some(Rc::clone(data)),
        }
    }

    pub(crate) fn serves(
        &self,
        command: &Command,
    ) -> bool {
        self.data.as_ref().is_none_or(|data| data.serves(command))
    }

    pub(crate) fn can_remove_folder(
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
        assert!(answers.serves(&Command::AddFolderToWorkspace));
        assert!(!answers.can_remove_folder("src"));
    }
}
