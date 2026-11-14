//! The hover buttons at the end of a tree row.

use ui::{IconName, IconWeight};

use crate::command::Command;
use crate::intent::Intent;
use crate::lock::Lock;
use crate::row::Row;
use crate::row_kind::RowKind;
use crate::workspace_status::WorkspaceStatus;
use crate::worktree::Worktree;

/// A button of a row's trailing cluster, in left to right order.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RowVerb {
    /// Move the branch's holder checkout to a detached HEAD.
    Detach,
    /// Retry provisioning a placeholder workspace.
    Retry,
    /// The repository menu, below the button.
    RepoMenu,
    /// Remove the chat, folder or branch.
    Remove,
    /// Start a thread under the row.
    Thread,
    /// Fork a branch.
    Fork,
    /// Make this thread a branch.
    MakeBranch,
}

impl RowVerb {
    /// The verbs `row` earns. A holder leaves Detach as the only remedy, a failed provision
    /// Retry; placeholders start nothing; a chat never forks but a repo thread can become a
    /// branch; the repository is removed through its menu and a locked branch not at all.
    pub fn for_row(row: &Row) -> Vec<Self> {
        let mut verbs = Vec::new();
        let placeholder = is_placeholder(row);
        let held = row
            .workspace
            .as_ref()
            .is_some_and(|workspace| workspace.held_by_path.is_some());
        if row.kind == RowKind::Branch && held {
            verbs.push(Self::Detach);
        }
        if row.kind == RowKind::Branch && placeholder && !held {
            verbs.push(Self::Retry);
        }
        if row.kind == RowKind::RepoHeader {
            verbs.push(Self::RepoMenu);
        }
        if is_removable(row) {
            verbs.push(Self::Remove);
        }
        if !placeholder {
            verbs.push(Self::Thread);
            verbs.extend(fork_and_branch(row));
        }
        verbs
    }

    pub(crate) fn icon(self) -> (IconName, IconWeight) {
        match self {
            Self::Detach => (IconName::LinkBreak, IconWeight::Bold),
            Self::Retry => (IconName::ArrowClockwise, IconWeight::Bold),
            Self::RepoMenu => (IconName::DotsThree, IconWeight::Bold),
            Self::Remove => (IconName::X, IconWeight::Bold),
            Self::Thread => (IconName::ArrowElbowDownRight, IconWeight::Bold),
            Self::Fork | Self::MakeBranch => (IconName::GitBranch, IconWeight::Bold),
        }
    }

    /// The accessible name of the button for `row`.
    pub fn label(self, row: &Row) -> String {
        let (label, branch) = (&row.label, branch_name(row));
        match self {
            Self::Detach => format!("Detach {branch}"),
            Self::Retry => format!("Retry {branch}"),
            Self::RepoMenu => format!("More actions for {label}"),
            Self::Remove if row.kind == RowKind::Branch => {
                format!("Remove branch {branch} and its worktree")
            },
            Self::Remove => format!("Remove {label}"),
            Self::Thread => format!("Thread {label}"),
            Self::Fork => format!("Fork {label}"),
            Self::MakeBranch => "Make this thread a branch".into(),
        }
    }

    /// What clicking does; the repo menu opens a popup the view owns, so it has no intent.
    pub fn intent(self, row: &Row) -> Option<Intent> {
        let id = row.id.clone();
        match self {
            Self::Detach => Some(Intent::Send(Command::Detach(id))),
            Self::Retry => Some(Intent::Send(Command::Retry(id))),
            Self::RepoMenu => None,
            Self::Remove => Some(Intent::RequestRemoval(id)),
            Self::Thread => Some(Intent::Send(Command::Thread(id))),
            Self::Fork => Some(Intent::StartDraft(id)),
            Self::MakeBranch => Some(Intent::StartBranching(id)),
        }
    }
}

fn branch_name(row: &Row) -> &str {
    row.workspace
        .as_ref()
        .map_or(row.label.as_str(), |workspace| workspace.branch.as_str())
}

fn is_placeholder(row: &Row) -> bool {
    row.workspace
        .as_ref()
        .is_some_and(|workspace| workspace.status == WorkspaceStatus::Placeholder)
}

fn is_removable(row: &Row) -> bool {
    match row.kind {
        RowKind::Chat | RowKind::Folder => true,
        RowKind::Branch => row.lock == Lock::Unlocked,
        RowKind::RepoHeader => false,
    }
}

fn fork_and_branch(row: &Row) -> Vec<RowVerb> {
    let mut verbs = Vec::new();
    let forks = match row.kind {
        RowKind::Folder => row.worktree == Worktree::Owned,
        RowKind::Branch => true,
        RowKind::Chat | RowKind::RepoHeader => false,
    };
    if forks {
        verbs.push(RowVerb::Fork);
    }
    if row.kind == RowKind::Chat && row.worktree == Worktree::Shared && row.pending_move.is_none() {
        verbs.push(RowVerb::MakeBranch);
    }
    verbs
}

#[cfg(test)]
#[path = "row_verb_tests.rs"]
mod tests;
