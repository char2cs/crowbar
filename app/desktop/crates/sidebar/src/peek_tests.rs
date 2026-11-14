use super::*;

#[test]
fn sliding_in_takes_two_hundred_milliseconds_and_reduced_motion_is_instant() {
    let mut peek = Peek::default();
    assert!(!peek.is_wanted(), "parked at start");
    peek.slide_to(1.0, Motion::default());
    assert!(peek.is_wanted(), "wanted after slide in");
    assert!(
        peek.is_moving(Motion::default()),
        "moving right after the request"
    );
    assert!(peek.position(Motion::default()) < 0.1, "barely started");
    let mut instant = Peek::default();
    instant.slide_to(1.0, Motion::reduced());
    assert!(instant.position(Motion::reduced()) < 0.1, "starts at zero");
}
