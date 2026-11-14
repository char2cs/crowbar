use super::*;

#[test]
fn the_threshold_is_euclidean() {
    assert!(
        !drag_started((0.0, 0.0), (5.0, 5.0)),
        "7.07px is not enough"
    );
    assert!(drag_started((0.0, 0.0), (6.0, 6.0)), "8.49px starts");
    assert!(
        drag_started((10.0, 10.0), (10.0, 18.0)),
        "exactly 8px starts"
    );
}
