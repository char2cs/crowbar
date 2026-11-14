use super::*;

#[test]
fn a_new_row_is_idle_final_and_closed() {
    let row = Row::new("c1", RowKind::Chat, 2, "Untitled chat");
    assert_eq!(row.id.as_str(), "c1", "id");
    assert_eq!(row.depth, 2, "depth");
    assert_eq!(row.activity, Activity::Idle, "idle");
    assert_eq!(row.presence, Presence::Absent, "closed");
    assert!(
        row.workspace.is_none() && row.operation.is_none(),
        "no workspace or operation"
    );
}

#[test]
fn two_lines_need_an_unlocked_workspace_with_another_name() {
    use crate::workspace::Workspace;
    use crate::workspace_status::WorkspaceStatus;
    let mut row = Row::new("c", RowKind::Chat, 1, "Untitled chat");
    assert!(!row.is_two_line(), "no workspace");
    row.workspace = Some(Workspace {
        status: WorkspaceStatus::New,
        branch: "inv-rb".into(),
        added: 1,
        deleted: 0,
        held_by_path: None,
    });
    assert!(row.is_two_line(), "title differs from the branch");
    row.label = "inv-rb".into();
    assert!(!row.is_two_line(), "same name");
    row.label = "x".into();
    row.lock = Lock::Locked;
    assert!(!row.is_two_line(), "locked rows stay one line");
}
