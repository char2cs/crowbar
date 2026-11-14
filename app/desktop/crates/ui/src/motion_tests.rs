use super::*;

#[test]
fn reduced_motion_collapses_durations() {
    let full = Motion::default();
    let reduced = Motion::reduced();
    assert_eq!(full.duration(Duration::from_millis(200)), Duration::from_millis(200));
    assert_eq!(reduced.duration(Duration::from_millis(200)), Duration::from_micros(10));
    assert!(reduced.is_reduced() && !full.is_reduced());
}

#[test]
fn animation_applies_the_curve() {
    let animation = Motion::default().animation(Duration::from_millis(150), CubicBezier::DEFAULT);
    assert_eq!(animation.duration, Duration::from_millis(150));
    assert!(animation.oneshot);
    let eased = (animation.easing)(0.5);
    assert!((eased - 0.77556).abs() < 2e-4, "{eased}");
}

#[test]
fn essential_loops_ignore_reduction() {
    let animation = Motion::essential_loop(Duration::from_secs(1), CubicBezier::LINEAR);
    assert_eq!(animation.duration, Duration::from_secs(1));
    assert!(!animation.oneshot);
}
