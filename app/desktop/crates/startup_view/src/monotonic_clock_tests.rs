use super::*;

#[test]
fn time_never_goes_backwards() {
    let clock = MonotonicClock::start();
    let first = clock.now();
    assert!(clock.now() >= first);
}
