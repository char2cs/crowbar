use super::*;

#[test]
fn respawn_delays_double_up_to_the_cap() {
    let base = Duration::from_millis(500);
    let cap = Duration::from_secs(10);
    let delays: Vec<Duration> = (1..=7).map(|n| recovery_delay(base, cap, n)).collect();
    assert_eq!(
        delays,
        [500, 1000, 2000, 4000, 8000, 10_000, 10_000].map(Duration::from_millis)
    );
    assert_eq!(recovery_delay(base, cap, 0), base);
    assert_eq!(recovery_delay(base, cap, usize::MAX), cap);
}

#[test]
fn a_daemon_that_reports_another_pid_than_the_one_started_is_named_stale() {
    let warning = pid_mismatch(Some(99), Some(7)).unwrap();
    assert!(
        warning.contains("99") && warning.contains('7') && warning.contains("stale"),
        "{warning}"
    );
}

#[test]
fn matching_or_unknown_pids_raise_no_warning() {
    for (reported, spawned) in [
        (Some(7), Some(7)),
        (None, Some(7)),
        (Some(7), None),
        (None, None),
    ] {
        assert_eq!(
            pid_mismatch(reported, spawned),
            None,
            "{reported:?} {spawned:?}"
        );
    }
}
