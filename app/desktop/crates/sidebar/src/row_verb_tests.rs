use super::*;
use crate::workspace::Workspace;

fn branch(status: WorkspaceStatus, held: Option<&str>) -> Row {
    let mut row = Row::new("b", RowKind::Branch, 1, "feature");
    row.workspace = Some(Workspace {
        status,
        branch: "feature".into(),
        added: 0,
        deleted: 0,
        held_by_path: held.map(str::to_owned),
    });
    row
}

#[test]
fn a_repo_thread_can_be_removed_threaded_and_branched() {
    let row = Row::new("c", RowKind::Chat, 2, "hi");
    assert_eq!(
        RowVerb::for_row(&row),
        [RowVerb::Remove, RowVerb::Thread, RowVerb::MakeBranch],
        "chat verbs"
    );
}

#[test]
fn a_chat_that_owns_a_worktree_cannot_branch_and_a_pending_move_blocks_it() {
    let mut owned = Row::new("c", RowKind::Chat, 2, "hi");
    owned.worktree = Worktree::Owned;
    assert_eq!(
        RowVerb::for_row(&owned),
        [RowVerb::Remove, RowVerb::Thread],
        "owned worktree"
    );
    let mut moving = Row::new("c", RowKind::Chat, 2, "hi");
    moving.pending_move = Some("src".into());
    assert_eq!(
        RowVerb::for_row(&moving),
        [RowVerb::Remove, RowVerb::Thread],
        "pending move"
    );
}

#[test]
fn repo_header_has_only_the_menu_and_thread() {
    let row = Row::new("r", RowKind::RepoHeader, 0, "demo");
    assert_eq!(
        RowVerb::for_row(&row),
        [RowVerb::RepoMenu, RowVerb::Thread],
        "repo verbs"
    );
}

#[test]
fn locked_branches_cannot_be_removed_and_holders_leave_detach() {
    let mut locked = branch(WorkspaceStatus::New, None);
    locked.lock = Lock::Locked;
    assert_eq!(
        RowVerb::for_row(&locked),
        [RowVerb::Thread, RowVerb::Fork],
        "locked branch"
    );
    let held = branch(WorkspaceStatus::New, Some("/tmp/x"));
    assert_eq!(RowVerb::for_row(&held)[0], RowVerb::Detach, "holder first");
}

#[test]
fn placeholders_offer_retry_and_nothing_that_needs_a_worktree() {
    let row = branch(WorkspaceStatus::Placeholder, None);
    assert_eq!(
        RowVerb::for_row(&row),
        [RowVerb::Retry, RowVerb::Remove],
        "placeholder verbs"
    );
}

#[test]
fn labels_and_intents_name_the_row() {
    let row = branch(WorkspaceStatus::New, None);
    assert_eq!(
        RowVerb::Remove.label(&row),
        "Remove branch feature and its worktree",
        "branch remove"
    );
    assert_eq!(RowVerb::Fork.label(&row), "Fork feature", "fork");
    assert_eq!(RowVerb::RepoMenu.intent(&row), None, "repo menu is a popup");
    assert_eq!(
        RowVerb::Fork.intent(&row),
        Some(Intent::StartDraft(row.id.clone())),
        "fork starts a draft"
    );
}
