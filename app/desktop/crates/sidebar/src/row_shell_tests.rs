use gpui::{ParentElement, TestAppContext};
use ui::{Surface, Theme};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn a_row_is_the_spec_height_plus_its_vertical_margin(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, cx| {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        row_shell("row".into(), 0, palette).child("label")
    });
    assert_eq!(f32::from(bounds.size.height), ROW_HEIGHT + ROW_MARGIN_Y);
}
