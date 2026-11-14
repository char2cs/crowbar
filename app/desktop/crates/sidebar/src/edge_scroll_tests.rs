use super::*;

#[test]
fn speed_ramps_from_fourteen_at_the_edge_to_one_at_the_band() {
    let cases = [
        (0.0, 14.0),
        (18.0, 7.0),
        (35.9, 1.0),
        (36.0, 0.0),
        (-1.0, 0.0),
        (100.0, 0.0),
    ];
    for (distance, expected) in cases {
        assert!(
            (edge_speed(distance) - expected).abs() < f32::EPSILON,
            "distance {distance}"
        );
    }
}
