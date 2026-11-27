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

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Row {
    pub id: RowId,
    pub kind: RowKind,
    pub depth: u8,
    pub label: String,
    pub titling: Titling,
    pub activity: Activity,
    pub nesting: Nesting,
    pub lock: Lock,
    pub presence: Presence,
    pub worktree: Worktree,
    pub workspace: Option<Workspace>,
    pub mark: Option<Mark>,
    pub pending_move: Option<String>,
    pub operation: Option<Operation>,
}

impl Row {
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
    pub fn is_two_line(&self) -> bool {
        self.lock == Lock::Unlocked
            && self
                .workspace
                .as_ref()
                .is_some_and(|workspace| workspace.branch != self.label)
    }

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
