//! A small in-memory shell for the gallery and tests: it serves a fixed workspace and applies the
//! commands the sidebar sends.

use std::cell::RefCell;

use crate::error::Error;
use crate::fixtures::rows::{reference_rows, rich_rows};
use crate::shell::Command;
use crate::shell::Focus;
use crate::shell::IconTarget;
use crate::shell::MergeBox;
use crate::shell::ProjectId;
use crate::shell::Row;
use crate::shell::RowId;
use crate::shell::RunningAgent;
use crate::shell::ScopeSync;
use crate::shell::Sink;
use crate::shell::Titling;

const RICH_COUNTS: (usize, usize) = (3, 2);

/// The fixed workspace of the gallery; rows can be renamed and removed through the sink.
#[derive(Debug)]
pub struct DemoShell {
    pub(crate) rows: RefCell<Vec<Row>>,
    pub(crate) active: RefCell<ProjectId>,
    pub(crate) spaces: usize,
    pub(crate) hosts: usize,
    pub(crate) sync: ScopeSync,
    pub(crate) focus: Focus,
    pub(crate) refused: Vec<Command>,
    pub(crate) removable: Vec<String>,
    pub(crate) merge: Option<MergeBox>,
    pub(crate) git_error: Option<String>,
    pub(crate) running: Vec<RunningAgent>,
    pub(crate) busy_icons: Vec<IconTarget>,
    pub(crate) adding: Vec<String>,
}

impl DemoShell {
    /// Two spaces and one host with a locked `main` and one provisional chat that owns a worktree,
    /// as in the reference capture.
    pub fn reference() -> Self {
        Self::with_rows(reference_rows(), (2, 1))
    }

    /// Several spaces and a tree that uses every row kind, state and badge.
    pub fn rich() -> Self {
        Self::with_rows(rich_rows(), RICH_COUNTS)
    }

    pub(crate) fn with_rows(
        rows: Vec<Row>,
        (spaces, hosts): (usize, usize),
    ) -> Self {
        Self {
            rows: RefCell::new(rows),
            active: RefCell::new(ProjectId::from("inv")),
            spaces,
            hosts,
            sync: ScopeSync::Ready,
            focus: Focus::Repository,
            refused: Vec::new(),
            removable: Vec::new(),
            merge: None,
            git_error: None,
            running: Vec::new(),
            busy_icons: Vec::new(),
            adding: Vec::new(),
        }
    }

    /// What the focused pane shows; only a repository enables every mode.
    #[must_use]
    pub fn with_focus(
        mut self,
        focus: Focus,
    ) -> Self {
        self.focus = focus;
        self
    }

    /// The same data with a shell that declines `command`, which hides its menu entries.
    #[must_use]
    pub fn refusing(
        mut self,
        command: Command,
    ) -> Self {
        self.refused.push(command);
        self
    }

    /// The same data with `path` as one of several folders of the workspace.
    #[must_use]
    pub fn with_removable_folder(
        mut self,
        path: &str,
    ) -> Self {
        self.removable.push(path.to_owned());
        self
    }

    /// The same data with `merge` as the merge box of the focused workspace.
    #[must_use]
    pub fn with_merge_box(
        mut self,
        merge: MergeBox,
    ) -> Self {
        self.merge = Some(merge);
        self
    }

    /// The same data with git refusing to read the focused workspace for `reason`.
    #[must_use]
    pub fn with_git_error(
        mut self,
        reason: &str,
    ) -> Self {
        self.git_error = Some(reason.to_owned());
        self
    }

    /// The reference workspace before the projects scope has synced, which shows the skeleton.
    pub fn loading() -> Self {
        Self {
            sync: ScopeSync::Pending,
            ..Self::reference()
        }
    }
}

fn already_exists(name: &str) -> Error {
    Error::Refused {
        reason: format!("\"{name}\" already exists"),
    }
}

impl DemoShell {
    fn rename(
        &self,
        row: &RowId,
        name: &str,
    ) -> Result<(), Error> {
        let mut rows = self.rows.borrow_mut();
        let taken = rows
            .iter()
            .any(|other| &other.id != row && other.label == name);
        if taken {
            return Err(already_exists(name));
        }
        for found in rows.iter_mut().filter(|other| &other.id == row) {
            name.clone_into(&mut found.label);
            found.titling = Titling::Final;
        }
        Ok(())
    }
}

impl Sink for DemoShell {
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error> {
        match command {
            Command::SelectProject(project) => *self.active.borrow_mut() = project,
            Command::Rename { row, name } => return self.rename(&row, &name),
            Command::CommitRemoval(row) | Command::RemoveDiscardingWork(row) => {
                self.rows.borrow_mut().retain(|other| other.id != row);
            },
            _ => {},
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::ActiveSpace as _;

    #[test]
    fn renaming_to_a_taken_name_is_refused_and_leaves_the_tree_alone() {
        let data = DemoShell::reference();
        let refused = data.dispatch(Command::Rename {
            row: RowId::from("chat"),
            name: "main".into(),
        });
        assert!(
            matches!(refused, Err(Error::Refused { .. })),
            "got {refused:?}"
        );
        let tree = data.tree(&ProjectId::from("inv"));
        assert!(
            tree.rows.iter().any(|row| row.label == "Untitled chat"),
            "label kept"
        );
    }

    #[test]
    fn committing_a_removal_drops_the_row() {
        let data = DemoShell::reference();
        data.dispatch(Command::CommitRemoval(RowId::from("chat")))
            .expect("accepted");
        assert!(
            data.tree(&ProjectId::from("inv"))
                .rows
                .iter()
                .all(|row| row.id != RowId::from("chat"))
        );
    }
}
