use gpui::{TestAppContext, px};

use super::*;
use crate::test_support::measure;

fn options() -> Vec<SharedString> {
    vec!["System".into(), "Light".into(), "Dark".into()]
}

#[gpui::test]
fn trigger_is_thirty_two_pixels_high_and_as_wide_as_requested(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| {
        Select::new("s", options(), Some(1)).width(px(160.0))
    });
    assert_eq!(
        (f32::from(bounds.size.width), f32::from(bounds.size.height)),
        (160.0, 32.0)
    );
}

#[gpui::test]
fn default_width_is_one_hundred_forty_four(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| {
        Select::new("s", options(), None).placeholder("Choose")
    });
    assert_eq!(f32::from(bounds.size.width), 144.0);
}

#[gpui::test]
fn compact_triggers_are_28px_tall(cx: &mut TestAppContext) {
    let compact = measure(cx, |_, _| {
        Select::new("s", vec!["A".into()], Some(0)).compact()
    });
    let standard = measure(cx, |_, _| Select::new("s", vec!["A".into()], Some(0)));
    assert_eq!(
        (
            f32::from(compact.size.height),
            f32::from(standard.size.height)
        ),
        (28.0, 32.0)
    );
}
