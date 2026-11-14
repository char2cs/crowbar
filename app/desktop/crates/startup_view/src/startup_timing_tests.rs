use super::*;

#[test]
fn default_reveals_late_and_holds_longer_than_it_reveals() {
    let timing = StartupTiming::default();
    assert_eq!(timing.reveal_delay(), StartupTiming::DEFAULT_REVEAL_DELAY);
    assert_eq!(timing.min_visible(), StartupTiming::DEFAULT_MIN_VISIBLE);
    assert!(timing.min_visible() > timing.reveal_delay());
}
