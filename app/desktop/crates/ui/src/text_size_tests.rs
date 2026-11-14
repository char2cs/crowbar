use super::*;

#[test]
fn ui_text_scales_with_the_setting() {
    const CASES: &[(UiText, f32, f32)] = &[
        (UiText::Xs, 15.0, 11.0),
        (UiText::Sm, 15.0, 12.0),
        (UiText::Base, 15.0, 14.0),
        (UiText::Sm, 30.0, 19.2),
        (UiText::Xl, 12.0, 16.0),
    ];
    for &(size, setting, expected) in CASES {
        let theme = Theme::default().with_ui_font_size(setting);
        let actual: f32 = size.size(&theme).into();
        assert!((actual - expected).abs() < 1e-4, "{size:?} at {setting}: {actual}");
    }
}
