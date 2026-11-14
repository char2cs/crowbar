//! The context menu of a tree row.

use ui::IconName;

use crate::command::Command;
use crate::intent::Intent;
use crate::lock::Lock;
use crate::menu_context::MenuContext;
use crate::menu_entry::MenuEntry;
use crate::row::Row;
use crate::row_kind::RowKind;
use crate::terminal_threads::TerminalThreads;
use crate::workspace_status::WorkspaceStatus;
use crate::worktree::Worktree;

/// The entries for a right click on `row`, top to bottom: rename, lock, import, creation,
/// moves, then the destructive group after a separator.
pub fn row_menu(row: &Row, context: &MenuContext) -> Vec<MenuEntry> {
    let id = &row.id;
    let mut entries = Vec::new();
    if !is_locked_branch(row) {
        let intent = Intent::StartRename(id.clone());
        entries.push(MenuEntry::action(
            "Rename",
            IconName::PencilSimpleLine,
            intent,
        ));
    }
    entries.extend(lock_entry(row));
    if row.kind == RowKind::RepoHeader {
        let intent = Intent::Send(Command::ImportBranches(id.clone()));
        entries.push(MenuEntry::action(
            "Import branches",
            IconName::DownloadSimple,
            intent,
        ));
    }
    entries.extend(creation_entries(row, context));
    entries.extend(move_entries(row, context));
    if row.kind == RowKind::RepoHeader {
        entries.push(MenuEntry::Separator);
        entries.push(MenuEntry::destructive(
            "Delete Repo",
            IconName::Trash,
            Intent::RequestRemoval(id.clone()),
        ));
    }
    entries
}

fn is_locked_branch(row: &Row) -> bool {
    row.kind == RowKind::Branch && row.lock == Lock::Locked
}

fn is_placeholder(row: &Row) -> bool {
    row.workspace
        .as_ref()
        .is_some_and(|workspace| workspace.status == WorkspaceStatus::Placeholder)
}

fn lock_entry(row: &Row) -> Option<MenuEntry> {
    if row.kind != RowKind::Branch || is_placeholder(row) {
        return None;
    }
    let id = row.id.clone();
    Some(match row.lock {
        Lock::Unlocked => {
            MenuEntry::action("Lock", IconName::Lock, Intent::Send(Command::Lock(id)))
        },
        Lock::Locked => MenuEntry::action(
            "Unlock",
            IconName::LockOpen,
            Intent::Send(Command::Unlock(id)),
        ),
    })
}

fn creation_entries(row: &Row, context: &MenuContext) -> Vec<MenuEntry> {
    if row.kind == RowKind::RepoHeader {
        return Vec::new();
    }
    let folder = Command::CreateFolder(Some(row.id.clone()));
    let mut entries = vec![MenuEntry::action(
        "New folder",
        IconName::Folder,
        Intent::Send(folder),
    )];
    if context.terminal == TerminalThreads::Available {
        let command = Command::TerminalThread(row.id.clone());
        entries.push(MenuEntry::action(
            "New thread in Terminal",
            IconName::Terminal,
            Intent::Send(command),
        ));
    }
    entries
}

fn move_entries(row: &Row, context: &MenuContext) -> Vec<MenuEntry> {
    let mut entries = Vec::new();
    if !context.targets.is_empty() {
        entries.push(MenuEntry::Submenu {
            label: "Move to…".into(),
            icon: Some(IconName::ArrowsOutCardinal),
            entries: move_targets(row, context),
        });
    }
    entries.extend(make_branch_entry(row));
    if row.pending_move.is_some() {
        let intent = Intent::Send(Command::CancelMove(row.id.clone()));
        entries.push(MenuEntry::action("Cancel move", IconName::XCircle, intent));
    }
    entries
}

fn move_targets(row: &Row, context: &MenuContext) -> Vec<MenuEntry> {
    context
        .targets
        .iter()
        .map(|target| {
            let icon = match target.kind {
                RowKind::Folder => IconName::Folder,
                _ => IconName::GitBranch,
            };
            let command = Command::MoveTo {
                row: row.id.clone(),
                target: target.id.clone(),
            };
            MenuEntry::action(&target.label, icon, Intent::Send(command))
        })
        .collect()
}

fn make_branch_entry(row: &Row) -> Option<MenuEntry> {
    let applies =
        row.kind == RowKind::Chat && row.worktree == Worktree::Shared && row.pending_move.is_none();
    let intent = Intent::StartBranching(row.id.clone());
    applies.then(|| MenuEntry::action("Make a branch", IconName::GitBranch, intent))
}

#[cfg(test)]
#[path = "row_menu_tests.rs"]
mod tests;
