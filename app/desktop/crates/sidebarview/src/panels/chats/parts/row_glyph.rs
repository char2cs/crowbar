//! Which glyph leads a tree row.

use ui::{IconName, IconWeight};

use sidebar::{Activity, Expansion, Lock, Row, RowKind, WorkspaceStatus, Worktree};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowGlyph {
    Spinner,
    Mark,
    Icon(IconName, IconWeight),
}

impl RowGlyph {
    pub(crate) fn of(
        row: &Row,
        expansion: Expansion,
    ) -> Self {
        if row.activity == Activity::Working {
            return Self::Spinner;
        }
        if row.kind == RowKind::RepoHeader {
            return Self::Mark;
        }
        if row.kind == RowKind::Folder {
            return Self::folder(expansion);
        }
        Self::branch_or_chat(row)
    }

    fn folder(expansion: Expansion) -> Self {
        let name = match expansion {
            Expansion::Expanded => IconName::FolderOpen,
            Expansion::Collapsed => IconName::Folder,
        };
        Self::Icon(name, IconWeight::Duotone)
    }

    fn branch_or_chat(row: &Row) -> Self {
        if let Some(workspace) = &row.workspace {
            let (name, weight) = status_glyph(workspace.status);
            return Self::Icon(name, weight);
        }
        if row.lock == Lock::Locked {
            return Self::Icon(IconName::Lock, IconWeight::Fill);
        }
        if row.worktree == Worktree::Owned {
            return Self::Icon(IconName::GitBranch, IconWeight::Fill);
        }
        Self::Icon(IconName::ChatsCircle, IconWeight::Regular)
    }
}

pub(crate) fn status_glyph(status: WorkspaceStatus) -> (IconName, IconWeight) {
    match status {
        WorkspaceStatus::Placeholder | WorkspaceStatus::PrConflicts => {
            (IconName::Warning, IconWeight::Fill)
        },
        WorkspaceStatus::Locked => (IconName::Lock, IconWeight::Fill),
        WorkspaceStatus::New => (IconName::GitBranch, IconWeight::Fill),
        WorkspaceStatus::Deleted | WorkspaceStatus::PrClosed => {
            (IconName::GitFork, IconWeight::Fill)
        },
        WorkspaceStatus::PrOpen => (IconName::GitPullRequest, IconWeight::Fill),
        WorkspaceStatus::PrMerged => (IconName::GitMerge, IconWeight::Fill),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Workspace;

    fn working_repo() -> Row {
        let mut row = Row::new("r", RowKind::RepoHeader, 0, "demo");
        row.activity = Activity::Working;
        row
    }

    fn folder() -> Row {
        Row::new("f", RowKind::Folder, 1, "src")
    }

    fn locked_branch() -> Row {
        let mut row = Row::new("b", RowKind::Branch, 1, "main");
        row.lock = Lock::Locked;
        row
    }

    fn worktree_branch() -> Row {
        let mut row = Row::new("b", RowKind::Branch, 1, "main");
        row.worktree = Worktree::Owned;
        row
    }

    fn merged_branch() -> Row {
        let mut row = worktree_branch();
        row.workspace = Some(Workspace {
            status: WorkspaceStatus::PrMerged,
            branch: "x".into(),
            added: 0,
            deleted: 0,
            held_by_path: None,
        });
        row
    }

    fn chat() -> Row {
        Row::new("c", RowKind::Chat, 1, "hi")
    }

    type Case = (&'static str, fn() -> Row, Expansion, RowGlyph);

    const CASES: &[Case] = &[
        (
            "working beats everything",
            working_repo,
            Expansion::Expanded,
            RowGlyph::Spinner,
        ),
        (
            "closed folder",
            folder,
            Expansion::Collapsed,
            RowGlyph::Icon(IconName::Folder, IconWeight::Duotone),
        ),
        (
            "open folder",
            folder,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::FolderOpen, IconWeight::Duotone),
        ),
        (
            "locked without workspace",
            locked_branch,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::Lock, IconWeight::Fill),
        ),
        (
            "owns a worktree",
            worktree_branch,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::GitBranch, IconWeight::Fill),
        ),
        (
            "the workspace status wins",
            merged_branch,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::GitMerge, IconWeight::Fill),
        ),
        (
            "a plain chat shows the bubbles",
            chat,
            Expansion::Expanded,
            RowGlyph::Icon(IconName::ChatsCircle, IconWeight::Regular),
        ),
    ];

    #[test]
    fn the_glyph_follows_activity_kind_status_lock_and_worktree() {
        for (name, build, expansion, expected) in CASES {
            assert_eq!(RowGlyph::of(&build(), *expansion), *expected, "{name}");
        }
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
            assert_eq!(status_glyph(status), (icon, IconWeight::Fill), "{status:?}");
            assert!(
                ui::Icon::path(icon, IconWeight::Fill).is_some(),
                "{icon:?} fill is embedded"
            );
        }
    }
}
