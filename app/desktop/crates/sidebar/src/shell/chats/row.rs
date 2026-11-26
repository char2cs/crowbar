//! One row of the chats tree as the data source derives it.

use crate::shell::chats::facets::Activity;
use crate::shell::chats::facets::Lock;
use crate::shell::chats::facets::Nesting;
use crate::shell::chats::facets::Operation;
use crate::shell::chats::facets::Presence;
use crate::shell::chats::facets::Titling;
use crate::shell::chats::facets::Workspace;
use crate::shell::chats::facets::Worktree;
use crate::shell::chats::mark::Mark;
use crate::shell::chats::row_id::RowId;
use crate::shell::chats::row_kind::RowKind;

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
    pub fn new(
        id: impl Into<RowId>,
        kind: RowKind,
        depth: u8,
        label: impl Into<String>,
    ) -> Self {
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

    /// Whether opening the row folds it instead of opening a chat: a folder, the repo home and a
    /// locked branch are containers, never checked out into.
    pub fn is_container(&self) -> bool {
        match self.kind {
            RowKind::Folder | RowKind::RepoHeader => true,
            RowKind::Branch => self.lock == Lock::Locked,
            RowKind::Chat => false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn two_lines_need_an_unlocked_workspace_with_another_name() {
        use crate::shell::chats::facets::Workspace;
        use crate::shell::chats::facets::WorkspaceStatus;
        let mut row = Row::new("c", RowKind::Chat, 1, "Untitled chat");
        assert!(!row.is_two_line(), "no workspace");
        row.workspace = Some(Workspace {
            status: WorkspaceStatus::New,
            branch: "inv-rb".into(),
            added: 1,
            deleted: 0,
            held_by_path: None,
        });
        assert!(row.is_two_line(), "title differs from the branch");
        row.label = "inv-rb".into();
        assert!(!row.is_two_line(), "same name");
        row.label = "x".into();
        row.lock = Lock::Locked;
        assert!(!row.is_two_line(), "locked rows stay one line");
    }
}
