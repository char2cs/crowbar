use super::*;
use crate::operation::Operation;
use crate::operation_phase::OperationPhase;

fn tree() -> Vec<Row> {
    let mut branch = Row::new("b1", RowKind::Branch, 1, "feature");
    branch.worktree = Worktree::Owned;
    vec![
        Row::new("r", RowKind::RepoHeader, 0, "demo"),
        branch,
        Row::new("c1", RowKind::Chat, 2, "one"),
        Row::new("f", RowKind::Folder, 1, "src"),
        Row::new("c2", RowKind::Chat, 2, "two"),
        Row::new("c3", RowKind::Chat, 1, "three"),
        Row::new("r2", RowKind::RepoHeader, 0, "other"),
        Row::new("f2", RowKind::Folder, 1, "docs"),
    ]
}

#[test]
fn chats_land_before_after_or_into_any_container() {
    let rows = tree();
    assert_eq!(
        allowed_modes(&rows, 2, 3),
        DropModes::ALL,
        "chat over folder"
    );
    assert_eq!(allowed_modes(&rows, 2, 0), DropModes::ALL, "chat over repo");
    assert_eq!(
        allowed_modes(&rows, 2, 5),
        DropModes::REORDER,
        "chat over chat only reorders"
    );
}

#[test]
fn a_row_never_lands_on_itself_or_inside_itself() {
    let rows = tree();
    assert_eq!(allowed_modes(&rows, 3, 3), DropModes::NONE, "self");
    assert_eq!(allowed_modes(&rows, 3, 4), DropModes::NONE, "own child");
    assert_eq!(
        allowed_modes(&rows, 0, 3),
        DropModes::NONE,
        "repo into its own folder"
    );
}

#[test]
fn busy_rows_refuse_both_ways() {
    let mut rows = tree();
    rows[3].operation = Some(Operation {
        label: "New workspace".into(),
        phase: OperationPhase::Running,
    });
    assert_eq!(allowed_modes(&rows, 2, 3), DropModes::NONE, "target busy");
    assert_eq!(allowed_modes(&rows, 3, 2), DropModes::NONE, "subject busy");
}

#[test]
fn a_working_chat_may_only_leave_into_a_container_and_working_folders_never_move() {
    let mut rows = tree();
    rows[2].activity = Activity::Working;
    assert_eq!(
        allowed_modes(&rows, 2, 3),
        DropModes::INTO,
        "chat waits for the turn"
    );
    assert_eq!(
        allowed_modes(&rows, 2, 5),
        DropModes::NONE,
        "no reorder while working"
    );
    rows[3].activity = Activity::Working;
    assert_eq!(
        allowed_modes(&rows, 3, 1),
        DropModes::NONE,
        "working folder"
    );
}

#[test]
fn repo_headers_only_reorder_among_repo_headers() {
    let rows = tree();
    assert_eq!(
        allowed_modes(&rows, 0, 6),
        DropModes::REORDER,
        "repo over repo"
    );
    assert_eq!(
        allowed_modes(&rows, 0, 5),
        DropModes::NONE,
        "repo over chat"
    );
}

#[test]
fn folders_and_branches_stay_inside_their_repository() {
    let rows = tree();
    assert_eq!(
        allowed_modes(&rows, 3, 1),
        DropModes::REORDER,
        "siblings reorder"
    );
    assert_eq!(
        allowed_modes(&rows, 3, 7),
        DropModes::NONE,
        "other repository"
    );
}

#[test]
fn a_branch_without_a_worktree_only_reorders() {
    let mut rows = tree();
    rows[1].worktree = Worktree::Shared;
    rows.push(Row::new("f3", RowKind::Folder, 2, "nested"));
    assert_eq!(
        allowed_modes(&rows, 1, 4),
        DropModes::NONE,
        "no nesting for a plain branch"
    );
}
