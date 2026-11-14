use super::*;
use crate::move_target::MoveTarget;
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

#[test]
fn repo_header_gets_import_and_a_destructive_tail() {
    let row = Row::new("r", RowKind::RepoHeader, 0, "demo");
    let entries = row_menu(&row, &MenuContext::default());
    assert_eq!(
        labels(&entries),
        ["Rename", "Import branches", "---", "Delete Repo"],
        "repo menu"
    );
}

#[test]
fn a_locked_branch_cannot_be_renamed_but_can_be_unlocked() {
    let mut row = Row::new("b", RowKind::Branch, 1, "main");
    row.lock = Lock::Locked;
    let entries = row_menu(&row, &MenuContext::default());
    assert_eq!(
        labels(&entries),
        ["Unlock", "New folder"],
        "locked branch menu"
    );
}

#[test]
fn placeholders_cannot_be_locked() {
    let mut row = Row::new("b", RowKind::Branch, 1, "wip");
    row.workspace = Some(workspace(WorkspaceStatus::Placeholder));
    assert_eq!(
        labels(&row_menu(&row, &MenuContext::default())),
        ["Rename", "New folder"],
        "no lock entry"
    );
}

#[test]
fn chats_offer_terminal_move_and_branching_in_order() {
    let row = Row::new("c", RowKind::Chat, 2, "hi");
    let context = MenuContext {
        targets: vec![MoveTarget {
            id: RowId::from("f"),
            label: "src".into(),
            kind: RowKind::Folder,
        }],
        terminal: TerminalThreads::Available,
    };
    let entries = row_menu(&row, &context);
    assert_eq!(
        labels(&entries),
        [
            "Rename",
            "New folder",
            "New thread in Terminal",
            "Move to…",
            "Make a branch"
        ],
        "chat menu"
    );
    let MenuEntry::Submenu {
        entries: targets, ..
    } = &entries[3]
    else {
        panic!("move to is a submenu");
    };
    assert_eq!(labels(targets), ["src"], "destinations");
}

#[test]
fn a_pending_move_replaces_make_a_branch_with_cancel() {
    let mut row = Row::new("c", RowKind::Chat, 2, "hi");
    row.pending_move = Some("src".into());
    let names = labels(&row_menu(&row, &MenuContext::default()));
    assert_eq!(
        names,
        ["Rename", "New folder", "Cancel move"],
        "pending move menu"
    );
}
