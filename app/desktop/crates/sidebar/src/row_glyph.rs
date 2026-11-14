//! Which glyph leads a tree row.

use ui::{IconName, IconWeight};

use crate::activity::Activity;
use crate::expansion::Expansion;
use crate::lock::Lock;
use crate::row::Row;
use crate::row_kind::RowKind;
use crate::worktree::Worktree;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowGlyph {
    Spinner,
    Mark,
    Icon(IconName, IconWeight),
}

impl RowGlyph {
    pub(crate) fn of(row: &Row, expansion: Expansion) -> Self {
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
            let (name, weight) = workspace.status.glyph();
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

#[cfg(test)]
#[path = "row_glyph_tests.rs"]
mod tests;
