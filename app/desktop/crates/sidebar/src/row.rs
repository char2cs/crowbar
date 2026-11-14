//! One row of the chats tree as the data source derives it.

use crate::activity::Activity;
use crate::lock::Lock;
use crate::mark::Mark;
use crate::nesting::Nesting;
use crate::operation::Operation;
use crate::presence::Presence;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::titling::Titling;
use crate::workspace::Workspace;
use crate::worktree::Worktree;

/// A row in tree order; `depth` places it, folded ancestors hide it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Row {
    /// Identity, stable across renames.
    pub id: RowId,
    /// What the row stands for.
    pub kind: RowKind,
    /// Nesting depth, zero at the top.
    pub depth: u8,
    /// The text shown.
    pub label: String,
    /// Provisional titles are italic.
    pub titling: Titling,
    /// Running turn or operation.
    pub activity: Activity,
    /// Has children.
    pub nesting: Nesting,
    /// Locked branch.
    pub lock: Lock,
    /// Whether a view is open for the row.
    pub presence: Presence,
    /// Whether the row owns a worktree.
    pub worktree: Worktree,
    /// The landed workspace of a branch.
    pub workspace: Option<Workspace>,
    /// The mark of a repo header.
    pub mark: Option<Mark>,
    /// Where a working chat will move after its turn.
    pub pending_move: Option<String>,
    /// An operation drawn in place of the row.
    pub operation: Option<Operation>,
}

impl Row {
    /// A plain idle row of `kind` at `depth`.
    pub fn new(id: impl Into<RowId>, kind: RowKind, depth: u8, label: impl Into<String>) -> Self {
        Self {
            id: id.into(),
            kind,
            depth,
            label: label.into(),
            titling: Titling::Final,
            activity: Activity::Idle,
            nesting: Nesting::Leaf,
            lock: Lock::Unlocked,
            presence: Presence::Absent,
            worktree: Worktree::Shared,
            workspace: None,
            mark: None,
            pending_move: None,
            operation: None,
        }
    }
}

impl Row {
    /// Whether the row shows the branch name and line counts under its title: a branch or chat
    /// owning an unlocked workspace whose branch is not its label.
    pub fn is_two_line(&self) -> bool {
        self.lock == Lock::Unlocked
            && self
                .workspace
                .as_ref()
                .is_some_and(|workspace| workspace.branch != self.label)
    }
}

#[cfg(test)]
#[path = "row_tests.rs"]
mod tests;
