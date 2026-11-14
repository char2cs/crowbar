use super::*;

#[test]
fn heights_match_the_spec() {
    const CASES: &[(ButtonSize, f32, f32, f32)] = &[
        (ButtonSize::Xs, 24.0, 6.0, 14.0),
        (ButtonSize::Sm, 28.0, 8.0, 16.0),
        (ButtonSize::Md, 32.0, 10.0, 16.0),
        (ButtonSize::Lg, 36.0, 12.0, 16.0),
        (ButtonSize::Xl, 40.0, 14.0, 18.0),
    ];
    for &(size, height, padding, icon) in CASES {
        assert_eq!(f32::from(size.height()), height, "{size:?} height");
        assert_eq!(f32::from(size.padding_x()), padding, "{size:?} padding");
        assert_eq!(f32::from(size.icon()), icon, "{size:?} icon");
    }
}
