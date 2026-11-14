use super::*;

const SPAN: Duration = Duration::from_millis(100);

fn linear() -> Tween {
    Tween::new(SPAN, CubicBezier::LINEAR)
}

#[test]
fn progress_follows_the_curve_and_settles() {
    let (start, running) = linear().at(Duration::ZERO, Motion::default());
    assert!(running && start.abs() < 1e-6);
    let (middle, running) = linear().at(Duration::from_millis(50), Motion::default());
    assert!(running && (middle - 0.5).abs() < 1e-3);
    assert_eq!(
        linear().at(Duration::from_millis(250), Motion::default()),
        (1.0, false)
    );
}

#[test]
fn delay_holds_progress_at_zero() {
    let tween = linear().delayed(Duration::from_millis(40));
    assert_eq!(
        tween.at(Duration::from_millis(39), Motion::default()),
        (0.0, true)
    );
    let (after, running) = tween.at(Duration::from_millis(90), Motion::default());
    assert!(running && (after - 0.5).abs() < 1e-3);
}

#[test]
fn reduced_motion_finishes_at_once() {
    let tween = linear().delayed(Duration::from_millis(40));
    assert_eq!(
        tween.at(Duration::from_millis(1), Motion::reduced()),
        (1.0, false)
    );
}
