use super::*;

fn hold_at(deadline: Deadline) -> Hold {
    Hold {
        row: RowId::from("c"),
        kind: RemovalKind::Chat,
        label: "chat".into(),
        deadline,
    }
}

#[test]
fn seconds_round_up_and_clockless_holds_have_none() {
    let now = Instant::now();
    let hold = hold_at(Deadline::At(now + Duration::from_millis(7_001)));
    assert_eq!(hold.seconds_left(now), Some(8), "7.001s shows 8");
    assert_eq!(
        hold.seconds_left(now + Duration::from_millis(7_001)),
        Some(0),
        "due shows 0"
    );
    assert_eq!(
        hold_at(Deadline::AwaitsConfirmation).seconds_left(now),
        None,
        "confirm has no clock"
    );
}

#[test]
fn due_only_at_or_after_the_deadline() {
    let now = Instant::now();
    let hold = hold_at(Deadline::At(now + Duration::from_secs(1)));
    assert!(!hold.is_due(now), "not yet");
    assert!(hold.is_due(now + Duration::from_secs(1)), "due");
}

#[test]
fn the_drain_fraction_tracks_the_real_deadline() {
    let now = Instant::now();
    let hold = hold_at(Deadline::At(now + Duration::from_secs(2)));
    let total = Duration::from_secs(8);
    assert!(
        (hold.drain(now, total) - 0.25).abs() < 0.001,
        "2s of 8s left"
    );
    assert!(
        hold.drain(now + Duration::from_secs(5), total).abs() < f32::EPSILON,
        "empty after the deadline"
    );
}
