//! The context menu of a tree row.

use sidebar::{
    Command, Intent, Lock, MenuContext, MoveTarget, Question, Row, RowKind, WorkspaceStatus,
    Worktree,
};
use ui::IconName;

use crate::overlays::MenuEntry;

pub(crate) fn row_menu(
    row: &Row,
    context: &MenuContext,
) -> Vec<MenuEntry> {
    let id = &row.id;
    let rename = (!is_locked_branch(row)).then(|| {
        let intent = Intent::StartRename(id.clone());
        MenuEntry::action("Rename", IconName::PencilSimpleLine, intent)
    });
    let is_repo = row.kind == RowKind::RepoHeader;
    let import = is_repo.then(|| {
        let intent = Intent::Send(Command::ImportBranches(id.clone()));
        MenuEntry::action("Import branches", IconName::DownloadSimple, intent)
    });
    let delete = is_repo.then(|| {
        [
            MenuEntry::Separator,
            MenuEntry::destructive(
                "Delete Repo",
                IconName::Trash,
                Intent::RequestRemoval(id.clone()),
            ),
        ]
    });
    rename
        .into_iter()
        .chain(lock_entry(row))
        .chain(import)
        .chain(std::iter::once(folder_entry(row)))
        .chain(move_entries(row, context))
        .chain(delete.into_iter().flatten())
        .collect()
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

fn folder_entry(row: &Row) -> MenuEntry {
    let folder = Command::CreateFolder(Some(row.id.clone()));
    MenuEntry::action("New folder", IconName::Folder, Intent::Send(folder))
}

fn move_entries(
    row: &Row,
    context: &MenuContext,
) -> Vec<MenuEntry> {
    let submenu = (!context.targets.is_empty()).then(|| move_submenu(row, context));
    let cancel = row.pending_move.is_some().then(|| {
        let intent = Intent::Send(Command::CancelMove(row.id.clone()));
        MenuEntry::action("Cancel move", IconName::XCircle, intent)
    });
    submenu
        .into_iter()
        .chain(make_branch_entry(row))
        .chain(cancel)
        .chain(kill_entries(context, &row.id).into_iter().flatten())
        .collect()
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

fn kill_entries(
    context: &MenuContext,
    row: &sidebar::RowId,
) -> Option<[MenuEntry; 2]> {
    let title = context.running.as_deref()?;
    let question = Question::kill(row.clone(), title);
    let kill = MenuEntry::destructive("Kill agent", IconName::Skull, Intent::Ask(question));
    Some([MenuEntry::Separator, kill])
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
    use sidebar::{RowId, Workspace};

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

    fn running_moving_chat() -> (Row, MenuContext) {
        let mut row = Row::new("c", RowKind::Chat, 2, "hi");
        row.pending_move = Some("src".into());
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
            "a repo header gets import, a new folder and a destructive tail",
            repo,
            &[
                "Rename",
                "Import branches",
                "New folder",
                "---",
                "Delete Repo",
            ],
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
            "chats offer move and branching in order",
            chat,
            &["Rename", "New folder", "Move to…", "Make a branch"],
        ),
        (
            "a running agent can be killed",
            running_chat,
            &["Rename", "New folder", "Make a branch", "---", "Kill agent"],
        ),
        (
            "cancel move comes before the kill, which sits apart",
            running_moving_chat,
            &["Rename", "New folder", "Cancel move", "---", "Kill agent"],
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
        } = &entries[2]
        else {
            panic!("move to is a submenu");
        };
        assert_eq!(labels(targets), ["src"], "destinations");
    }
}
