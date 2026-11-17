//! The rows of the demo data: the reference capture and a richer tree.

use crate::activity::Activity;
use crate::avatar_tint::AvatarTint;
use crate::lock::Lock;
use crate::mark::Mark;
use crate::nesting::Nesting;
use crate::presence::Presence;
use crate::row::Row;
use crate::row_kind::RowKind;
use crate::titling::Titling;
use crate::workspace::Workspace;
use crate::workspace_status::WorkspaceStatus;
use crate::worktree::Worktree;

pub(crate) const MAIN: &str = "main";
const SYNC_LOOP_ADDED: u32 = 42;
const SYNC_LOOP_DELETED: u32 = 7;
const SPIKE_ADDED: u32 = 3;
const SPIKE_DELETED: u32 = 1;

fn branch(
    id: &str,
    depth: u8,
    label: &str,
    workspace: Workspace,
) -> Row {
    let mut row = Row::new(id, RowKind::Branch, depth, label);
    row.workspace = Some(workspace);
    row
}

fn landed(
    status: WorkspaceStatus,
    branch: &str,
    added: u32,
    deleted: u32,
) -> Workspace {
    Workspace {
        status,
        branch: branch.to_owned(),
        added,
        deleted,
        held_by_path: None,
    }
}

fn repo_header(
    id: &str,
    label: &str,
    letter: char,
    tint: AvatarTint,
) -> Row {
    let mut row = Row::new(id, RowKind::RepoHeader, 0, label);
    row.mark = Some(Mark::Letter { letter, tint });
    row.nesting = Nesting::Parent;
    row
}

pub(crate) fn reference_rows() -> Vec<Row> {
    let mut main = branch(MAIN, 1, MAIN, landed(WorkspaceStatus::Locked, MAIN, 0, 0));
    main.lock = Lock::Locked;
    let mut chat = Row::new("chat", RowKind::Branch, 1, "Untitled chat");
    chat.titling = Titling::Provisional;
    chat.presence = Presence::Open;
    chat.worktree = Worktree::Owned;
    chat.workspace = Some(landed(WorkspaceStatus::New, "inv-rb", 1, 0));
    vec![
        repo_header("demo", "demo", 'D', AvatarTint::Slate),
        main,
        chat,
    ]
}

pub(crate) fn rich_rows() -> Vec<Row> {
    let mut rows = reference_rows();
    let mut working = Row::new("working", RowKind::Chat, 1, "Refactor the sync loop");
    working.activity = Activity::Working;
    working.workspace = Some(landed(
        WorkspaceStatus::PrOpen,
        "sync-loop",
        SYNC_LOOP_ADDED,
        SYNC_LOOP_DELETED,
    ));
    let mut folder = Row::new("folder", RowKind::Folder, 1, "Experiments");
    folder.nesting = Nesting::Parent;
    let child = branch(
        "child",
        2,
        "spike",
        landed(
            WorkspaceStatus::PrMerged,
            "spike",
            SPIKE_ADDED,
            SPIKE_DELETED,
        ),
    );
    rows.extend([working, folder, child]);
    rows
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_rich_tree_holds_the_reference_rows_and_more() {
        let reference = reference_rows();
        let rich = rich_rows();
        assert!(
            reference
                .iter()
                .all(|row| rich.iter().any(|other| other.id == row.id))
        );
        assert!(rich.len() > reference.len());
    }
}
