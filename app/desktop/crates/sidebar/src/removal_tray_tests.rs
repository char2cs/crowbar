use super::*;

fn chat(id: &str) -> (RowId, (RemovalKind, String)) {
    (RowId::from(id), (RemovalKind::Chat, id.to_owned()))
}

#[test]
fn chats_count_down_eight_seconds_and_repos_wait_for_confirmation() {
    let now = Instant::now();
    let mut tray = RemovalTray::default();
    let (id, info) = chat("c");
    tray.hold(id.clone(), info, now);
    tray.hold(RowId::from("r"), (RemovalKind::Repo, "demo".into()), now);
    assert_eq!(
        tray.get(&id).and_then(|hold| hold.seconds_left(now)),
        Some(8),
        "chat counts down"
    );
    assert_eq!(
        tray.get(&RowId::from("r")).map(|hold| hold.deadline),
        Some(Deadline::AwaitsConfirmation),
        "repo asks"
    );
    assert_eq!(
        tray.next_deadline(),
        Some(now + RemovalTray::DRAIN),
        "one timer"
    );
}

#[test]
fn keep_restores_the_row_and_due_holds_are_sent_once() {
    let now = Instant::now();
    let mut tray = RemovalTray::default();
    for id in ["a", "b"] {
        let (row, info) = chat(id);
        tray.hold(row, info, now);
    }
    tray.keep(&RowId::from("a"));
    assert!(
        tray.get(&RowId::from("a")).is_none(),
        "kept row is no longer held"
    );
    assert!(tray.take_due(now).is_empty(), "nothing due yet");
    let due = tray.take_due(now + RemovalTray::DRAIN);
    assert_eq!(due, vec![RowId::from("b")], "b is sent");
    assert!(
        tray.take_due(now + RemovalTray::DRAIN).is_empty(),
        "never sent twice"
    );
    assert!(!tray.is_counting(), "no countdown left");
}

#[test]
fn tombstones_release_committed_holds() {
    let now = Instant::now();
    let mut tray = RemovalTray::default();
    let (row, info) = chat("c");
    tray.hold(row.clone(), info, now);
    tray.commit(&row);
    tray.release_gone(|id| id.as_str() != "c");
    assert!(tray.holds().is_empty(), "released when the row is gone");
}
