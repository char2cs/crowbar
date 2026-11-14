use gpui::{TestAppContext, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn track_has_the_requested_width_and_a_fixed_height(cx: &mut TestAppContext) {
    for fraction in [None, Some(0.0), Some(0.5), Some(2.0)] {
        let bounds = measure(cx, move |_, _| {
            ProgressBar::new("bar", fraction).width(px(120.0))
        });
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (120.0, 6.0),
            "{fraction:?}"
        );
    }
}

#[test]
fn fractions_are_clamped_and_nan_is_empty() {
    const CASES: &[(f32, f32)] = &[(-1.0, 0.0), (0.25, 0.25), (3.0, 1.0), (f32::NAN, 0.0)];
    for &(input, expected) in CASES {
        assert_eq!(clamp_fraction(input), expected, "{input}");
    }
}
