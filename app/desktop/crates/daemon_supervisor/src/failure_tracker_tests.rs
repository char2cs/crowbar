use super::*;

#[test]
fn only_an_unserved_probe_counts_toward_a_wedge() {
    let mut tracker = FailureTracker::new(2);
    assert!(!tracker.observe(Probe::Unserved));
    assert!(
        !tracker.observe(Probe::Held),
        "a held daemon clears the streak"
    );
    assert!(!tracker.observe(Probe::Unserved));
    assert!(!tracker.observe(Probe::LocalDescriptorExhaustion));
    assert!(!tracker.observe(Probe::Unserved));
    assert!(tracker.observe(Probe::Unserved), "two in a row trip it");
    assert!(!tracker.observe(Probe::Healthy));
}

#[test]
fn a_trip_fires_once_per_wedge_and_can_fire_again_after_a_reset() {
    let mut tracker = FailureTracker::new(3);
    assert!(!tracker.observe(Probe::Unserved));
    assert!(!tracker.observe(Probe::Unserved));
    assert!(tracker.observe(Probe::Unserved));
    assert!(
        !tracker.observe(Probe::Unserved),
        "further failures do not re-trip"
    );
    tracker.reset();
    assert!(!tracker.observe(Probe::Unserved));
    assert!(!tracker.observe(Probe::Unserved));
    assert!(tracker.observe(Probe::Unserved));
}
