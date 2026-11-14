use super::*;

#[test]
fn sizes_match_the_spec() {
    const CASES: &[(BadgeSize, f32, f32, f32)] = &[
        (BadgeSize::Sm, 16.0, 10.0, 4.0),
        (BadgeSize::Md, 18.0, 12.0, 6.0),
        (BadgeSize::Lg, 22.0, 14.0, 6.0),
    ];
    for &(size, extent, text, radius) in CASES {
        assert_eq!(f32::from(size.extent()), extent, "{size:?}");
        assert_eq!(f32::from(size.text()), text, "{size:?}");
        assert_eq!(f32::from(size.radius()), radius, "{size:?}");
    }
}
