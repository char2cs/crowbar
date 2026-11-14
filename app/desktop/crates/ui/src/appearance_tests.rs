use super::*;

#[test]
fn window_appearance_maps_to_two_schemes() {
    const CASES: &[(WindowAppearance, Appearance)] = &[
        (WindowAppearance::Light, Appearance::Light),
        (WindowAppearance::VibrantLight, Appearance::Light),
        (WindowAppearance::Dark, Appearance::Dark),
        (WindowAppearance::VibrantDark, Appearance::Dark),
    ];
    for &(input, expected) in CASES {
        assert_eq!(Appearance::from(input), expected, "{input:?}");
    }
}
