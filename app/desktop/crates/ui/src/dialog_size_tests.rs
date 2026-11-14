use super::*;

#[test]
fn widths_match_the_spec() {
    const CASES: &[(DialogSize, f32)] =
        &[(DialogSize::Sm, 384.0), (DialogSize::Md, 448.0), (DialogSize::Lg, 672.0), (DialogSize::Xl, 896.0)];
    for &(size, width) in CASES {
        assert_eq!(f32::from(size.max_width()), width, "{size:?}");
    }
}
