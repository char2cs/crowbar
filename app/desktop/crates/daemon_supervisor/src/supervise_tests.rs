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
