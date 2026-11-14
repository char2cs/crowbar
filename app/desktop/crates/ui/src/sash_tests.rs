use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn hit_area_is_six_pixels_thick(cx: &mut TestAppContext) {
    let horizontal = measure(cx, |_, _| Sash::new("a", Axis::Horizontal));
    assert_eq!(f32::from(horizontal.size.width), 6.0);
    let vertical = measure(cx, |_, _| Sash::new("b", Axis::Vertical));
    assert_eq!(f32::from(vertical.size.height), 6.0);
    assert_eq!(f32::from(Sash::thickness()), 6.0);
}
