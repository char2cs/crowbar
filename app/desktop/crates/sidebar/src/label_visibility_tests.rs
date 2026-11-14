use super::*;

#[test]
fn thresholds_apply_to_the_bar_width() {
    let cases = [
        (250.0, LabelVisibility::Hidden),
        (294.0, LabelVisibility::Hidden),
        (296.0, LabelVisibility::ActiveOnly),
        (435.0, LabelVisibility::ActiveOnly),
        (436.0, LabelVisibility::All),
        (640.0, LabelVisibility::All),
    ];
    for (width, expected) in cases {
        assert_eq!(
            LabelVisibility::for_sidebar(width),
            expected,
            "width {width}"
        );
    }
}
