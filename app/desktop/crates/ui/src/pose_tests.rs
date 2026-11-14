use super::*;

#[test]
fn converts_both_ways_with_bool() {
    assert_eq!(Pose::from(true), Pose::On);
    assert_eq!(Pose::from(false), Pose::Off);
    assert!(bool::from(Pose::On));
    assert!(!bool::from(Pose::Off));
}

#[test]
fn rest_value_is_zero_or_one() {
    assert!((Pose::Off.rest() - 0.0).abs() < f32::EPSILON);
    assert!((Pose::On.rest() - 1.0).abs() < f32::EPSILON);
}
