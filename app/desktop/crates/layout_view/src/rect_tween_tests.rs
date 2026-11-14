use std::time::{Duration, Instant};

use model::Rect;
use ui::Motion;

use super::*;

const A: Rect = Rect {
    x: 0.0,
    y: 0.0,
    width: 100.0,
    height: 100.0,
};
const B: Rect = Rect {
    x: 50.0,
    y: 0.0,
    width: 50.0,
    height: 100.0,
};

#[test]
fn the_rectangle_ends_exactly_on_the_target_after_a_hundred_milliseconds() {
    let start = Instant::now();
    let mut tween = RectTween::resting(A);
    tween.go_to(B, start, Motion::default());
    let end = start + Duration::from_millis(100);
    assert_eq!(tween.rect(end, Motion::default()), B, "arrived");
    assert!(!tween.is_running(end, Motion::default()), "stopped");
}

#[test]
fn it_is_between_the_two_rectangles_while_moving() {
    let start = Instant::now();
    let mut tween = RectTween::resting(A);
    tween.go_to(B, start, Motion::default());
    let middle = tween.rect(start + Duration::from_millis(50), Motion::default());
    assert!(
        middle.x > 0.0 && middle.x < 50.0,
        "x in between: {}",
        middle.x
    );
    assert!(
        tween.is_running(start + Duration::from_millis(50), Motion::default()),
        "running"
    );
}
