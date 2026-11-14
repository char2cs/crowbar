use super::*;
use crate::workspace::Workspace;
use crate::workspace_status::WorkspaceStatus;

fn glyph(row: &Row) -> RowGlyph {
    RowGlyph::of(row, Expansion::Expanded)
}

#[test]
fn working_beats_everything() {
    let mut row = Row::new("r", RowKind::RepoHeader, 0, "demo");
    row.activity = Activity::Working;
    assert_eq!(glyph(&row), RowGlyph::Spinner, "spinner replaces the glyph");
}

#[test]
fn folders_follow_their_expansion() {
    let row = Row::new("f", RowKind::Folder, 1, "src");
    assert_eq!(
        RowGlyph::of(&row, Expansion::Collapsed),
        RowGlyph::Icon(IconName::Folder, IconWeight::Duotone),
        "closed folder"
    );
    assert_eq!(
        RowGlyph::of(&row, Expansion::Expanded),
        RowGlyph::Icon(IconName::FolderOpen, IconWeight::Duotone),
        "open folder"
    );
}

#[test]
fn branches_show_their_workspace_status_then_lock_then_worktree() {
    let mut row = Row::new("b", RowKind::Branch, 1, "main");
    row.lock = Lock::Locked;
    assert_eq!(
        glyph(&row),
        RowGlyph::Icon(IconName::Lock, IconWeight::Fill),
        "locked without workspace"
    );
    row.worktree = Worktree::Owned;
    row.lock = Lock::Unlocked;
    assert_eq!(
        glyph(&row),
        RowGlyph::Icon(IconName::GitBranch, IconWeight::Fill),
        "owns worktree"
    );
    row.workspace = Some(Workspace {
        status: WorkspaceStatus::PrMerged,
        branch: "x".into(),
        added: 0,
        deleted: 0,
        held_by_path: None,
    });
    assert_eq!(
        glyph(&row),
        RowGlyph::Icon(IconName::GitMerge, IconWeight::Fill),
        "status wins"
    );
}

#[test]
fn a_plain_chat_shows_the_bubbles() {
    let row = Row::new("c", RowKind::Chat, 1, "hi");
    assert_eq!(
        glyph(&row),
        RowGlyph::Icon(IconName::ChatsCircle, IconWeight::Regular),
        "chat"
    );
}
