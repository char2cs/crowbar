//! The glyph an idle chat or branch row leads with, shared with the view tab naming that chat.

use sidebar::{Lock, Row, WorkspaceStatus, Worktree};
use ui::{IconName, IconWeight};

/// The icon an idle chat or branch row leads with: its workspace's status, a lock, a branch or
/// the chat bubble. A view tab naming the same chat shows the same glyph.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ChatGlyph {
    /// The icon drawn.
    pub icon: IconName,
    /// The weight it is drawn at.
    pub weight: IconWeight,
}

impl ChatGlyph {
    /// The glyph for `row`, ignoring whether it is working: a spinner replaces it then.
    pub fn of(row: &Row) -> Self {
        if let Some(workspace) = &row.workspace {
            return Self::fill(status_icon(workspace.status));
        }
        if row.lock == Lock::Locked {
            return Self::fill(IconName::Lock);
        }
        if row.worktree == Worktree::Owned {
            return Self::fill(IconName::GitBranch);
        }
        Self {
            icon: IconName::ChatsCircle,
            weight: IconWeight::Regular,
        }
    }

    fn fill(icon: IconName) -> Self {
        Self {
            icon,
            weight: IconWeight::Fill,
        }
    }
}

fn status_icon(status: WorkspaceStatus) -> IconName {
    match status {
        WorkspaceStatus::Placeholder | WorkspaceStatus::PrConflicts => IconName::Warning,
        WorkspaceStatus::Locked => IconName::Lock,
        WorkspaceStatus::New => IconName::GitBranch,
        WorkspaceStatus::Deleted | WorkspaceStatus::PrClosed => IconName::GitFork,
        WorkspaceStatus::PrOpen => IconName::GitPullRequest,
        WorkspaceStatus::PrMerged => IconName::GitMerge,
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Activity, RowKind, Workspace};

    use super::*;

    fn branch_in(status: WorkspaceStatus) -> Row {
        let mut row = Row::new("b", RowKind::Branch, 1, "main");
        row.workspace = Some(Workspace {
            status,
            branch: "x".into(),
            added: 0,
            deleted: 0,
            held_by_path: None,
        });
        row
    }

    #[test]
    fn a_working_chat_still_names_its_idle_glyph() {
        let mut row = branch_in(WorkspaceStatus::PrMerged);
        row.activity = Activity::Working;
        let merged = ChatGlyph {
            icon: IconName::GitMerge,
            weight: IconWeight::Fill,
        };
        assert_eq!(ChatGlyph::of(&row), merged);
    }

    #[test]
    fn a_plain_chat_shows_the_bubble() {
        let row = Row::new("c", RowKind::Chat, 1, "hi");
        assert_eq!(ChatGlyph::of(&row).icon, IconName::ChatsCircle);
    }

    #[test]
    fn every_status_has_the_spec_glyph() {
        let cases = [
            (WorkspaceStatus::Placeholder, IconName::Warning),
            (WorkspaceStatus::Locked, IconName::Lock),
            (WorkspaceStatus::New, IconName::GitBranch),
            (WorkspaceStatus::PrConflicts, IconName::Warning),
            (WorkspaceStatus::Deleted, IconName::GitFork),
            (WorkspaceStatus::PrClosed, IconName::GitFork),
            (WorkspaceStatus::PrOpen, IconName::GitPullRequest),
            (WorkspaceStatus::PrMerged, IconName::GitMerge),
        ];
        for (status, icon) in cases {
            let row = branch_in(status);
            assert_eq!(ChatGlyph::of(&row), ChatGlyph::fill(icon), "{status:?}");
            assert!(
                ui::Icon::path(icon, IconWeight::Fill).is_some(),
                "{icon:?} fill is embedded"
            );
        }
    }
}
