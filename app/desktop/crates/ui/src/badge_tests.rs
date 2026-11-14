use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn height_follows_the_size_step(cx: &mut TestAppContext) {
    const CASES: &[(BadgeSize, f32)] = &[(BadgeSize::Sm, 16.0), (BadgeSize::Md, 18.0), (BadgeSize::Lg, 22.0)];
    for &(size, height) in CASES {
        let bounds = measure(cx, move |_, _| Badge::new("3").size(size));
        assert_eq!(f32::from(bounds.size.height), height, "{size:?}");
        assert!(f32::from(bounds.size.width) >= height, "{size:?} keeps its minimum width");
    }
}

#[gpui::test]
fn every_variant_renders(cx: &mut TestAppContext) {
    for variant in [
        BadgeVariant::Primary,
        BadgeVariant::Destructive,
        BadgeVariant::Error,
        BadgeVariant::Info,
        BadgeVariant::Outline,
        BadgeVariant::Secondary,
        BadgeVariant::Success,
        BadgeVariant::Warning,
    ] {
        let bounds = measure(cx, move |_, _| Badge::new("x").variant(variant));
        assert!(f32::from(bounds.size.width) > 0.0, "{variant:?}");
    }
}
