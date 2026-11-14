use super::*;

#[test]
fn end_and_middle_follow_from_x_and_width() {
    let span = Span {
        x: 10.0,
        width: 40.0,
    };
    assert_eq!(span.end(), 50.0, "right edge");
    assert_eq!(span.middle(), 30.0, "centre");
}
