use std::time::{Duration, Instant};

use super::*;

const SPAN: Duration = Duration::from_millis(100);

fn glide(value: f32) -> Glide {
    Glide::resting(value, SPAN, CubicBezier::LINEAR)
}

#[test]
fn a_resting_glide_holds_its_value_and_is_not_running() {
    let now = Instant::now();
    let rest = glide(3.0);
    assert_eq!(rest.value(now, Motion::default()), 3.0, "value at rest");
    assert!(
        !rest.is_running(now, Motion::default()),
        "not running at rest"
    );
}

#[test]
fn a_glide_reaches_halfway_halfway_through_and_finishes_at_the_target() {
    let start = Instant::now();
    let mut moving = glide(0.0);
    moving.go_to(10.0, start, Motion::default());
    let half = start + SPAN / 2;
    assert!(
        (moving.value(half, Motion::default()) - 5.0).abs() < 1e-3,
        "halfway"
    );
    assert!(
        moving.is_running(half, Motion::default()),
        "still running at half"
    );
    let end = start + SPAN;
    assert_eq!(moving.value(end, Motion::default()), 10.0, "arrived");
    assert!(
        !moving.is_running(end, Motion::default()),
        "stopped on arrival"
    );
}

#[test]
fn retargeting_starts_from_the_value_shown_at_that_moment() {
    let start = Instant::now();
    let mut moving = glide(0.0);
    moving.go_to(10.0, start, Motion::default());
    let half = start + SPAN / 2;
    moving.go_to(0.0, half, Motion::default());
    assert!(
        (moving.value(half, Motion::default()) - 5.0).abs() < 1e-3,
        "no jump"
    );
    assert_eq!(moving.target(), 0.0, "new target");
}

#[test]
fn the_same_target_does_not_restart_the_motion() {
    let start = Instant::now();
    let mut moving = glide(0.0);
    moving.go_to(10.0, start, Motion::default());
    let later = start + SPAN / 2;
    moving.go_to(10.0, later, Motion::default());
    assert!(
        (moving.value(later, Motion::default()) - 5.0).abs() < 1e-3,
        "kept its course"
    );
}

#[test]
fn reduced_motion_arrives_at_once() {
    let start = Instant::now();
    let mut moving = glide(0.0);
    moving.go_to(10.0, start, Motion::reduced());
    let soon = start + Duration::from_millis(1);
    assert_eq!(
        moving.value(soon, Motion::reduced()),
        10.0,
        "instant under reduced motion"
    );
}

#[test]
fn snap_jumps_without_motion() {
    let now = Instant::now();
    let mut moving = glide(0.0);
    moving.go_to(10.0, now, Motion::default());
    moving.snap(2.0);
    assert_eq!(moving.value(now, Motion::default()), 2.0, "snapped value");
    assert!(
        !moving.is_running(now, Motion::default()),
        "snap stops motion"
    );
}
