use gpui::{AppContext as _, ParentElement as _, Styled as _, TestAppContext, px};

use super::*;
use crate::test_support::measure;
use crate::{IconName, TextInput};

#[test]
fn heights_follow_the_style() {
    const CASES: &[(TextInputStyle, f32)] = &[
        (TextInputStyle::Standard, 32.0),
        (TextInputStyle::Compact, 28.0),
        (TextInputStyle::Stepper, 24.0),
        (TextInputStyle::Bare, 20.0),
    ];
    for &(style, height) in CASES {
        assert_eq!(style.height(), px(height), "{style:?}");
    }
}

#[test]
fn only_the_stepper_is_centred() {
    assert!(TextInputStyle::Stepper.centered());
    assert!(!TextInputStyle::Compact.centered());
}

#[gpui::test]
fn the_rendered_field_has_the_style_height(cx: &mut TestAppContext) {
    const CASES: &[(TextInputStyle, f32)] = &[
        (TextInputStyle::Compact, 28.0),
        (TextInputStyle::Stepper, 24.0),
    ];
    for &(style, height) in CASES {
        let bounds = measure(cx, move |_, cx| {
            let input = cx.new(|cx| {
                TextInput::new("x", cx)
                    .style(style)
                    .leading_icon(IconName::MagnifyingGlass)
            });
            gpui::div().w(px(120.0)).child(input)
        });
        assert_eq!(f32::from(bounds.size.height), height, "{style:?}");
    }
}
