use super::*;

const WIDTH_CASES: &[(usize, f32, f32)] = &[
    (1, 900.0, 240.0),
    (3, 900.0, 240.0),
    (4, 900.0, 225.0),
    (8, 900.0, 112.5),
    (20, 900.0, 112.0),
    (0, 900.0, 240.0),
];

#[test]
fn tabs_share_the_strip_equally_between_112_and_240() {
    for (count, available, expected) in WIDTH_CASES {
        assert_eq!(
            slot_width(*count, *available),
            *expected,
            "{count} tabs in {available}"
        );
    }
}

#[test]
fn slots_are_laid_end_to_end_from_zero() {
    let laid = slots(3, 900.0);
    let starts: Vec<f32> = laid.iter().map(|span| span.x).collect();
    assert_eq!(starts, vec![0.0, 240.0, 480.0], "starts");
}

#[test]
fn the_strip_scrolls_faster_the_closer_the_pointer_is_to_an_end() {
    let strip = Span {
        x: 0.0,
        width: 600.0,
    };
    assert_eq!(edge_scroll_step(300.0, strip), 0.0, "middle");
    assert_eq!(edge_scroll_step(0.0, strip), -14.0, "left edge");
    assert_eq!(edge_scroll_step(600.0, strip), 14.0, "right edge");
    assert!(
        edge_scroll_step(30.0, strip).abs() < edge_scroll_step(5.0, strip).abs(),
        "ramps up"
    );
    assert_eq!(
        edge_scroll_step(35.0, strip),
        -1.0,
        "never below one pixel inside the band"
    );
}
