//! The context menu of a tree row.

use ui::IconName;

use crate::command::Command;
use crate::intent::Intent;
use crate::lock::Lock;
use crate::menu_context::MenuContext;
use crate::menu_entry::MenuEntry;
use crate::move_target::MoveTarget;
use crate::question::Question;
use crate::row::Row;
use crate::row_kind::RowKind;
use crate::terminal_threads::TerminalThreads;
use crate::workspace_status::WorkspaceStatus;
use crate::worktree::Worktree;

pub(crate) fn row_menu(
    row: &Row,
    context: &MenuContext,
) -> Vec<MenuEntry> {
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

fn creation_entries(
    row: &Row,
    context: &MenuContext,
) -> Vec<MenuEntry> {
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

fn move_entries(
    row: &Row,
    context: &MenuContext,
) -> Vec<MenuEntry> {
    let mut entries = Vec::new();
    if !context.targets.is_empty() {
        entries.push(move_submenu(row, context));
    }
    entries.extend(make_branch_entry(row));
    entries.extend(kill_entry(row, context));
    if row.pending_move.is_some() {
        let intent = Intent::Send(Command::CancelMove(row.id.clone()));
        entries.push(MenuEntry::action("Cancel move", IconName::XCircle, intent));
    }
    entries
}

fn move_targets(
    row: &Row,
    context: &MenuContext,
) -> Vec<MenuEntry> {
    let entry = |target: &MoveTarget| move_entry(row, target);
    context.targets.iter().map(entry).collect()
}

fn move_entry(
    row: &Row,
    target: &MoveTarget,
) -> MenuEntry {
    let icon = match target.kind {
        RowKind::Folder => IconName::Folder,
        _ => IconName::GitBranch,
    };
    let command = Command::MoveTo {
        row: row.id.clone(),
        target: target.id.clone(),
    };
    MenuEntry::action(&target.label, icon, Intent::Send(command))
}

fn move_submenu(
    row: &Row,
    context: &MenuContext,
) -> MenuEntry {
    MenuEntry::Submenu {
        label: "Move to…".into(),
        icon: Some(IconName::ArrowsOutCardinal),
        entries: move_targets(row, context),
    }
}

fn kill_entry(
    row: &Row,
    context: &MenuContext,
) -> Option<MenuEntry> {
    let title = context.running.as_deref()?;
    let question = Question::kill(row.id.clone(), title);
    Some(MenuEntry::action(
        "Kill agent",
        IconName::XCircle,
        Intent::Ask(question),
    ))
}

fn make_branch_entry(row: &Row) -> Option<MenuEntry> {
    let applies =
        row.kind == RowKind::Chat && row.worktree == Worktree::Shared && row.pending_move.is_none();
    let intent = Intent::StartBranching(row.id.clone());
    applies.then(|| MenuEntry::action("Make a branch", IconName::GitBranch, intent))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::row_id::RowId;
    use crate::workspace::Workspace;

    fn labels(entries: &[MenuEntry]) -> Vec<String> {
        entries
            .iter()
            .map(|entry| match entry {
                MenuEntry::Action { label, .. } | MenuEntry::Submenu { label, .. } => label.clone(),
                MenuEntry::Separator => "---".into(),
            })
            .collect()
    }

    fn workspace(status: WorkspaceStatus) -> Workspace {
        Workspace {
            status,
            branch: "b".into(),
            added: 0,
            deleted: 0,
            held_by_path: None,
        }
    }

    fn repo() -> (Row, MenuContext) {
        let row = Row::new("r", RowKind::RepoHeader, 0, "demo");
        (row, MenuContext::default())
    }

    fn locked_branch() -> (Row, MenuContext) {
        let mut row = Row::new("b", RowKind::Branch, 1, "main");
        row.lock = Lock::Locked;
        (row, MenuContext::default())
    }

    fn placeholder() -> (Row, MenuContext) {
        let mut row = Row::new("b", RowKind::Branch, 1, "wip");
        row.workspace = Some(workspace(WorkspaceStatus::Placeholder));
        (row, MenuContext::default())
    }

    fn chat() -> (Row, MenuContext) {
        let row = Row::new("c", RowKind::Chat, 2, "hi");
        let target = MoveTarget {
            id: RowId::from("f"),
            label: "src".into(),
            kind: RowKind::Folder,
        };
        let context = MenuContext {
            targets: vec![target],
            terminal: TerminalThreads::Available,
            running: None,
        };
        (row, context)
    }

    fn moving_chat() -> (Row, MenuContext) {
        let mut row = Row::new("c", RowKind::Chat, 2, "hi");
        row.pending_move = Some("src".into());
        (row, MenuContext::default())
    }

    fn running_chat() -> (Row, MenuContext) {
        let row = Row::new("c", RowKind::Chat, 2, "hi");
        let context = MenuContext {
            running: Some("hi".into()),
            ..MenuContext::default()
        };
        (row, context)
    }

    type Case = (
        &'static str,
        fn() -> (Row, MenuContext),
        &'static [&'static str],
    );

    const CASES: &[Case] = &[
        (
            "a repo header gets import and a destructive tail",
            repo,
            &["Rename", "Import branches", "---", "Delete Repo"],
        ),
        (
            "a locked branch cannot be renamed but can be unlocked",
            locked_branch,
            &["Unlock", "New folder"],
        ),
        (
            "placeholders cannot be locked",
            placeholder,
            &["Rename", "New folder"],
        ),
        (
            "chats offer terminal, move and branching in order",
            chat,
            &[
                "Rename",
                "New folder",
                "New thread in Terminal",
                "Move to…",
                "Make a branch",
            ],
        ),
        (
            "a running agent can be killed",
            running_chat,
            &["Rename", "New folder", "Make a branch", "Kill agent"],
        ),
        (
            "a pending move replaces make a branch with cancel",
            moving_chat,
            &["Rename", "New folder", "Cancel move"],
        ),
    ];

    #[test]
    fn each_row_kind_gets_its_menu_in_order() {
        for (name, build, expected) in CASES {
            let (row, context) = build();
            assert_eq!(labels(&row_menu(&row, &context)), *expected, "{name}");
        }
    }

    #[test]
    fn move_to_lists_the_destinations() {
        let (row, context) = chat();
        let entries = row_menu(&row, &context);
        let MenuEntry::Submenu {
            entries: targets, ..
        } = &entries[3]
        else {
            panic!("move to is a submenu");
        };
        assert_eq!(labels(targets), ["src"], "destinations");
    }
}
