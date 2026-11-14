use std::time::Duration;

use super::*;
use crate::lock::Lock;

fn rows() -> Vec<Row> {
    let mut repo = Row::new("repo", RowKind::RepoHeader, 0, "demo");
    repo.nesting = Nesting::Parent;
    let mut main = Row::new("main", RowKind::Branch, 1, "main");
    main.nesting = Nesting::Parent;
    vec![
        repo,
        main,
        Row::new("chat", RowKind::Chat, 2, "Untitled"),
        Row::new("other", RowKind::Chat, 1, "Other"),
    ]
}

fn indices(state: &TreeState, rows: &[Row]) -> Vec<usize> {
    state.visible(rows).iter().map(|row| row.index).collect()
}

#[test]
fn folding_hides_descendants_only() {
    let rows = rows();
    let mut state = TreeState::default();
    assert_eq!(indices(&state, &rows), vec![0, 1, 2, 3], "all shown");
    state.toggle_fold(&RowId::from("main"));
    assert_eq!(indices(&state, &rows), vec![0, 1, 3], "main's chat hidden");
    state.toggle_fold(&RowId::from("repo"));
    assert_eq!(
        indices(&state, &rows),
        vec![0],
        "everything under the repo hidden"
    );
    state.toggle_fold(&RowId::from("repo"));
    state.toggle_fold(&RowId::from("main"));
    assert_eq!(
        indices(&state, &rows),
        vec![0, 1, 2, 3],
        "unfolding restores"
    );
}

#[test]
fn rename_and_branching_exclude_each_other_and_locked_branches_refuse() {
    let rows = rows();
    let mut state = TreeState::default();
    state.start_rename(&rows[2]);
    assert_eq!(
        state.editing(),
        &Editing::Renaming(RowId::from("chat")),
        "renaming"
    );
    state.start_branching(&RowId::from("other"));
    assert_eq!(
        state.editing(),
        &Editing::Branching(RowId::from("other")),
        "branching replaced it"
    );
    let mut locked = rows[1].clone();
    locked.lock = Lock::Locked;
    state.stop_editing();
    state.start_rename(&locked);
    assert_eq!(state.editing(), &Editing::Nothing, "locked branch refuses");
}

#[test]
fn a_draft_unfolds_its_parent() {
    let rows = rows();
    let mut state = TreeState::default();
    state.toggle_fold(&rows[1].id);
    state.start_draft(&rows[1].id);
    assert_eq!(indices(&state, &rows), vec![0, 1, 2, 3], "parent unfolded");
    assert_eq!(
        state.editing(),
        &Editing::Drafting(RowId::from("main")),
        "draft open"
    );
}

#[test]
fn a_chat_removal_counts_down_keeps_its_slot_and_hides_children() {
    let rows = rows();
    let now = Instant::now();
    let mut state = TreeState::default();
    state.request_removal(&rows[1], now);
    let visible = state.visible(&rows);
    assert_eq!(
        visible.iter().map(|row| row.index).collect::<Vec<_>>(),
        vec![0, 1, 3],
        "children hide"
    );
    assert!(
        visible[1].removal.is_some(),
        "the held row is drawn as a countdown"
    );
    state.keep(&rows[1].id);
    assert_eq!(
        indices(&state, &rows),
        vec![0, 1, 2, 3],
        "keep restores exactly"
    );
}

#[test]
fn due_removals_are_sent_and_then_hidden_until_the_tombstone() {
    let rows = rows();
    let now = Instant::now();
    let mut state = TreeState::default();
    state.request_removal(&rows[3], now);
    assert!(
        state.take_due(now + Duration::from_secs(1)).is_empty(),
        "too early"
    );
    let due = state.take_due(now + RemovalTray::DRAIN);
    assert_eq!(due, vec![RowId::from("other")], "sent");
    assert_eq!(
        indices(&state, &rows),
        vec![0, 1, 2],
        "committed row hidden"
    );
    state.reconcile(&rows[..3]);
    assert!(
        state.tray().holds().is_empty(),
        "tombstone releases the hold"
    );
}

#[test]
fn repositories_ask_and_cancelling_brings_the_row_back() {
    let rows = rows();
    let mut state = TreeState::default();
    state.request_removal(&rows[0], Instant::now());
    assert_eq!(
        state.confirmation().map(Confirmation::title),
        Some("Delete repository “demo”?".into()),
        "asks"
    );
    state.cancel_confirmation();
    assert!(
        state.confirmation().is_none() && state.tray().holds().is_empty(),
        "cancelled"
    );
    state.request_removal(&rows[0], Instant::now());
    let accepted = state.accept_confirmation();
    assert_eq!(
        accepted.map(|confirmation| confirmation.target),
        Some(RemovalTarget::Row(RowId::from("repo"))),
        "accepted"
    );
    assert_eq!(
        indices(&state, &rows),
        Vec::<usize>::new(),
        "repo and children hidden after sending"
    );
}

#[test]
fn a_refused_delete_asks_again_naming_the_loss() {
    let rows = rows();
    let mut state = TreeState::default();
    state.request_removal(&rows[2], Instant::now());
    let risk = WorkAtRisk {
        branch: "b".into(),
        uncommitted_files: 2,
        unmerged_commits: 0,
    };
    state.ask_to_discard(&rows[2].id, vec![risk]);
    let title = state.confirmation().map(Confirmation::title);
    assert_eq!(
        title,
        Some("Delete chat “Untitled” and lose work?".into()),
        "second question"
    );
    assert!(!state.tray().is_counting(), "no clock while asking");
}

#[test]
fn locked_branches_cannot_be_removed() {
    let mut locked = rows()[1].clone();
    locked.lock = Lock::Locked;
    let mut state = TreeState::default();
    state.request_removal(&locked, Instant::now());
    assert!(state.tray().holds().is_empty(), "no hold");
}

#[test]
fn spaces_ask_without_a_row() {
    let mut state = TreeState::default();
    state.request_project_removal(ProjectId::from("p"), "inv");
    assert_eq!(
        state.confirmation().map(Confirmation::title),
        Some("Delete space “inv”?".into()),
        "asks"
    );
}
