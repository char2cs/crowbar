use super::*;

#[test]
fn widths_match_the_spec_table() {
    const CASES: &[(ControlWidth, f32)] = &[
        (ControlWidth::Compact, 112.0),
        (ControlWidth::Default, 144.0),
        (ControlWidth::Wide, 176.0),
        (ControlWidth::ExtraWide, 224.0),
        (ControlWidth::NumberCompact, 96.0),
        (ControlWidth::Text, 192.0),
        (ControlWidth::TextWide, 224.0),
    ];
    for &(width, pixels) in CASES {
        assert_eq!(width.pixels(), pixels, "{width:?}");
    }
}
